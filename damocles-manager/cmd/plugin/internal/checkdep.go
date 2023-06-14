package internal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/urfave/cli/v2"
	"golang.org/x/mod/modfile"
)

var CheckDepCmd = &cli.Command{
	Name:    "check-dep",
	Usage:   "Check damocles-manager plugin dependencies",
	Aliases: []string{"checkdep"},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "damocles-branch",
			Usage:       "damocles branch or tag",
			DefaultText: "main",
			Aliases:     []string{"branch", "b", "venus-cluster-branch"},
		},
		&cli.BoolFlag{
			Name:  "fix",
			Usage: "Attempt to automatically fix dependencies",
		},
		&cli.StringFlag{
			Name:        "goc",
			Usage:       "the go compiler",
			DefaultText: "go",
		},
	},
	ArgsUsage: "<path of go.mod>",
	Action: func(cctx *cli.Context) error {
		if cctx.NArg() != 1 {
			return fmt.Errorf("incorrect number of arguments, got %d", cctx.NArg())
		}
		localGoModPath := cctx.Args().First()
		damoclesBranch := cctx.String("damocles-branch")
		if damoclesBranch == "" {
			damoclesBranch = "main"
		}

		if !cctx.Bool("fix") {
			diffs, err := diffDep(localGoModPath, damoclesBranch)
			if err != nil {
				return err
			}
			for _, diff := range diffs {
				fmt.Fprintf(os.Stderr, "%s %s => %s\n", diff.Path, diff.Version, diff.damoclesManagerVersion)
			}
			if len(diffs) != 0 {
				return fmt.Errorf("the versions of dependencies is inconsistent with damoclesManager")
			}
			return nil
		}

		goc := cctx.String("goc")
		if goc == "" {
			goc = "go"
		}

		ok, err := fixDep(cctx.Context, goc, localGoModPath, damoclesBranch)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("unable to automatically repair dependencies, please try to fix manually. (Run this command without `--fix` flag to get the dependencies version difference)")
		}
		return nil
	},
}

type DiffDep struct {
	Path                   string
	Version                string
	damoclesManagerVersion string
}

func diffDep(goModPath, damoclesBranch string) (diffs []DiffDep, err error) {
	localMod, err := loadLocalMod(goModPath)
	if err != nil {
		err = fmt.Errorf("load local go.mod: %w", err)
		return
	}
	damoclesManagerDeps, err := loadDamoclesManagerDeps(damoclesBranch)
	if err != nil {
		return
	}
	for _, require := range localMod.Require {
		if damoclesManagerVersion, ok := damoclesManagerDeps[require.Mod.Path]; ok {
			if require.Mod.Version != damoclesManagerVersion {
				diffs = append(diffs, DiffDep{
					Path:                   require.Mod.Path,
					Version:                require.Mod.Version,
					damoclesManagerVersion: damoclesManagerVersion,
				})
			}
		}
	}
	return
}

// Attempt to fix dependencies
func fixDep(ctx context.Context, goc, goModPath, damoclesManagerBranch string) (bool, error) {
	backup, err := backupGoMod(goModPath)
	if err != nil {
		return false, fmt.Errorf("backup go.mod: %w", err)
	}
	defer os.Remove(backup)

	localMod, err := loadLocalMod(goModPath)
	if err != nil {
		return false, fmt.Errorf("load local go.mod: %w", err)
	}
	damoclesManagerDeps, err := loadDamoclesManagerDeps(damoclesManagerBranch)
	if err != nil {
		return false, err
	}
	changed := applyVersion(localMod, damoclesManagerDeps)
	if !changed {
		return true, nil
	}
	newModData, err := localMod.Format()
	if err != nil {
		return false, fmt.Errorf("format mod file:%w", err)
	}
	fi, err := os.Stat(goModPath)
	if err != nil {
		return false, fmt.Errorf("stat go.mod: %w", err)
	}
	err = os.WriteFile(goModPath, newModData, fi.Mode())
	if err != nil {
		return false, fmt.Errorf("write data to go.mod: %w", err)
	}
	tidyCmd := exec.CommandContext(ctx, goc, "mod", "tidy")
	tidyCmd.Dir = path.Dir(goModPath)
	tidyCmd.Stderr = os.Stderr
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Env = append(os.Environ(), "GO111MODULE=on")
	err = tidyCmd.Run()
	if err != nil {
		return false, fmt.Errorf("go mod tidy plugin source code failure: %w", err)
	}
	localMod, err = loadLocalMod(goModPath)
	if err != nil {
		return false, fmt.Errorf("load local go.mod after tidy: %w", err)
	}

	// check the dependencies again after running `go mod tidy`
	changed = applyVersion(localMod, damoclesManagerDeps)
	if changed {
		// Automatic fix dependencies failed, restore backup file
		if err := (func() error {
			if err := os.Remove(goModPath); err != nil {
				return err
			}
			if err := os.Rename(backup, goModPath); err != nil {
				return err
			}
			return nil
		})(); err != nil {
			return false, fmt.Errorf("restore backup file: %w", err)
		}
		return false, nil
	}
	return true, nil
}

func applyVersion(localMod *modfile.File, damoclesManagerDeps map[string]string) (changed bool) {
	for _, require := range localMod.Require {
		if damoclesManagerVersion, ok := damoclesManagerDeps[require.Mod.Path]; ok {
			if require.Mod.Version != damoclesManagerVersion {
				changed = true
				require.Mod.Version = damoclesManagerVersion
			}
		}
	}
	if changed {
		localMod.SetRequire(localMod.Require)
	}
	return
}

func loadDamoclesManagerDeps(branch string) (map[string]string, error) {
	damoclesManagerMod, err := loadDamoclesManagerMod(branch)
	if err != nil {
		return nil, fmt.Errorf("load damocles-manager go.mod: %w", err)
	}

	damoclesManagerDeps := make(map[string]string)
	for _, require := range damoclesManagerMod.Require {
		damoclesManagerDeps[require.Mod.Path] = require.Mod.Version
	}
	return damoclesManagerDeps, nil
}

func loadLocalMod(goModPath string) (*modfile.File, error) {
	localGoModData, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("read %s file: %w", goModPath, err)
	}
	mod, err := modfile.Parse(goModPath, localGoModData, nil)
	if err != nil {
		return nil, fmt.Errorf("parse go.mod: %w", err)
	}
	return mod, nil
}

func loadDamoclesManagerMod(branch string) (*modfile.File, error) {
	modData, err := downloadDamoclesGoMod(branch)
	if err != nil {
		return nil, fmt.Errorf("download damocles-manager go.mod: %w", err)
	}
	mod, err := modfile.Parse("damocles-manager/go.mod", modData, nil)
	if err != nil {
		return nil, fmt.Errorf("parse damocles-manager go.mod: %w", err)
	}
	return mod, nil
}

func downloadDamoclesGoMod(ref string) ([]byte, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/ipfs-force-community/damocles/%s/damocles-manager/go.mod", ref)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the status code of the response is not ok. status code: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// backupGoMod backs up the go.mod file and returns the backup path
func backupGoMod(goModPath string) (string, error) {
	dirPath, filename := path.Split(goModPath)

	generateRandomString := func() string {
		randomBytes := make([]byte, 28)
		rand.Read(randomBytes)
		return hex.EncodeToString(randomBytes)
	}

	backupName := fmt.Sprintf(".%s.bak", filename)
	backupPath := path.Join(dirPath, backupName)
	for {
		if _, err := os.Stat(backupPath); errors.Is(err, os.ErrNotExist) {
			break
		}
		backupName = fmt.Sprintf(".%s.%s.bak", generateRandomString(), filename)
		backupPath = path.Join(dirPath, backupName)
	}

	fi, err := os.Stat(goModPath)
	if err != nil {
		return "", fmt.Errorf("stat go.mod: %w", err)
	}
	bytes, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	if err := os.WriteFile(backupPath, bytes, fi.Mode()); err != nil {
		return "", fmt.Errorf("write backup file: %w", err)
	}
	return backupPath, nil
}

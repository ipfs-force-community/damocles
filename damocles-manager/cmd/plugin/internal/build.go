package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"
)

var BuildCmd = &cli.Command{
	Name:  "build",
	Usage: "Build damocles-manager plugin",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "src-dir",
			Usage:    "plugin source folder path",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "out-dir",
			Usage:    "plugin packaged folder path",
			Required: true,
		},
		&cli.StringFlag{
			Name:        "goc",
			Usage:       "the go compiler",
			DefaultText: "go",
		},
	},
	Action: func(cctx *cli.Context) error {
		srcDir, err := filepath.Abs(cctx.String("src-dir"))
		if err != nil {
			return fmt.Errorf("unable to resolve absolute representation of src-dir path: %w", err)
		}
		outDir, err := filepath.Abs(cctx.String("out-dir"))
		if err != nil {
			return fmt.Errorf("unable to resolve absolute representation of out-dir path: %w", err)
		}
		goc := cctx.String("goc")
		if goc == "" {
			goc = "go"
		}
		return goBuild(cctx.Context, goc, srcDir, outDir)
	},
}

const codeTemplate = `
package main

import (
	managerplugin "github.com/ipfs-force-community/damocles/manager-plugin"
)

func PluginManifest() *managerplugin.Manifest {
	return managerplugin.ExportManifest(&managerplugin.{{.kind}}Manifest{
		Manifest: managerplugin.Manifest{
			Kind:           managerplugin.{{.kind}},
			Name:           "{{.name}}",
			Description:    "{{.description}}",
			BuildTime:      "{{.buildTime}}",
			{{if .onInit }}
				OnInit:     {{.onInit}},
			{{end}}
			{{if .onShutdown }}
				OnShutdown: {{.onShutdown}},
			{{end}}
		},
		{{range .export}}
		{{.extPoint}}: {{.impl}},
		{{end}}
	})
}
`

func goBuild(ctx context.Context, goc, srcDir, outDir string) error {
	var manifest map[string]interface{}
	_, err := toml.DecodeFile(filepath.Join(srcDir, "manifest.toml"), &manifest)
	if err != nil {
		return fmt.Errorf("read pkg %s's manifest failure: %w", srcDir, err)
	}
	manifest["buildTime"] = time.Now().Format(time.RFC3339)

	pluginName := manifest["name"].(string)
	tmpl, err := template.New("gen-plugin").Parse(codeTemplate)
	if err != nil {
		return fmt.Errorf("generate code failure during parse template: %w", err)
	}

	genFileName := filepath.Join(srcDir, filepath.Base(srcDir)+".gen.go")
	genFile, err := os.OpenFile(genFileName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0700) // #nosec G302
	if err != nil {
		return fmt.Errorf("generate code failure during prepare output file: %w", err)
	}
	defer func() {
		if err := os.Remove(genFileName); err != nil {
			log.Printf("remove tmp file %s failure, please clean up manually at %v", genFileName, err)
		}
	}()

	err = tmpl.Execute(genFile, manifest)
	if err != nil {
		return fmt.Errorf("generate code failure during generating code, %w", err)
	}

	outputFile := filepath.Join(outDir, "plugin-"+pluginName+".so")
	buildCmd := exec.CommandContext(ctx, goc, "build",
		"-buildmode=plugin",
		"-o", outputFile, srcDir)
	buildCmd.Dir = srcDir
	buildCmd.Stderr = os.Stderr
	buildCmd.Stdout = os.Stdout
	buildCmd.Env = append(os.Environ(), "GO111MODULE=on")
	err = buildCmd.Run()
	if err != nil {
		return fmt.Errorf("compile plugin source code failure, %w", err)
	}
	fmt.Printf(`Package "%s" as plugin "%s" success.`+"\nManifest:\n", srcDir, outputFile)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent(" ", "\t")
	err = encoder.Encode(manifest)
	if err != nil {
		return fmt.Errorf("print manifest detail failure, err: %w", err)
	}
	return nil
}

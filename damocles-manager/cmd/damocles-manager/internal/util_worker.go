package internal

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/workercli"
)

var utilWorkerCmd = &cli.Command{
	Name:  "worker",
	Flags: []cli.Flag{},
	Usage: "Utils for worker management",
	Subcommands: []*cli.Command{
		utilWorkerListCmd,
		utilWorkerRemoveCmd,
		utilWorkerInfoCmd,
		utilWorkerPauseCmd,
		utilWorkerResumeCmd,
		utilWdPostCmd,
	},
}

var utilWorkerListCmd = &cli.Command{
	Name: "list",
	Flags: []cli.Flag{
		&cli.DurationFlag{
			Name:  "expiration",
			Value: 10 * time.Minute,
			Usage: "timeout for regarding a woker as missing",
		},
	},
	Action: func(cctx *cli.Context) error {
		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		pinfos, err := a.Damocles.WorkerPingInfoList(actx)
		if err != nil {
			return RPCCallError("WorkerPingInfoList", err)
		}

		expiration := cctx.Duration("expiration")

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		defer tw.Flush()
		_, _ = fmt.Fprintln(tw, "Name\tDest\tVersion\tThreads\tEmpty\tPaused\tErrors\tLastPing(with ! if expired)")
		for _, pinfo := range pinfos {
			lastPing := time.Since(time.Unix(pinfo.LastPing, 0))
			lastPingWarn := ""
			if lastPing > expiration {
				lastPingWarn = " (!)"
			}

			_, _ = fmt.Fprintf(
				tw, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s%s\n",
				pinfo.Info.Name,
				pinfo.Info.Dest,
				pinfo.Info.Version,
				pinfo.Info.Summary.Threads,
				pinfo.Info.Summary.Empty,
				pinfo.Info.Summary.Paused,
				pinfo.Info.Summary.Errors,
				lastPing,
				lastPingWarn,
			)
		}

		return nil
	},
}

var utilWorkerRemoveCmd = &cli.Command{
	Name:      "remove",
	Usage:     "Remove the specific worker",
	ArgsUsage: "<worker instance name>",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 1 {
			return cli.ShowSubcommandHelp(cctx)
		}
		name := args.First()

		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		workerInfo, err := a.Damocles.WorkerGetPingInfo(actx, name)
		if err != nil {
			return RPCCallError("WorkerGetPingInfo", err)
		}

		if workerInfo == nil {
			return fmt.Errorf("worker info not found. please make sure the instance name is correct: %s", name)
		}

		if err = a.Damocles.WorkerPingInfoRemove(actx, name); err != nil {
			return err
		}
		fmt.Printf("'%s' removed\n", name)
		return nil
	},
}

var utilWorkerInfoCmd = &cli.Command{
	Name:      "info",
	Usage:     "Show details about the specific worker",
	ArgsUsage: "<worker instance name or address>",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 1 {
			return cli.ShowSubcommandHelp(cctx)
		}

		name := args.First()

		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		dest, err := resolveWorkerDest(actx, a, name)
		if err != nil {
			return fmt.Errorf("resolve worker dest: %w", err)
		}

		wcli, wstop, err := workercli.Connect(context.Background(), fmt.Sprintf("http://%s/", dest))
		if err != nil {
			return fmt.Errorf("connect to %s: %w", dest, err)
		}

		defer wstop()

		// use context.Background to avoid meta info
		details, err := wcli.WorkerList()
		if err != nil {
			return RPCCallError("WorkerList", err)
		}

		if len(details) == 0 {
			return nil
		}

		tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		defer tw.Flush()
		_, _ = fmt.Fprintln(tw, "Index\tLoc\tPlan\tSectorID\tPaused\tPausedElapsed\tState\tLastErr")

		for _, detail := range details {
			_, _ = fmt.Fprintf(
				tw, "%d\t%s\t%s\t%s\t%v\t%s\t%s\t%s\n",
				detail.Index,
				detail.Location,
				detail.Plan,
				FormatOrNull(detail.SectorID, func() string { return util.FormatSectorID(*detail.SectorID) }),
				detail.Paused,
				FormatOrNull(detail.PausedElapsed, func() string { return (time.Duration(*detail.PausedElapsed) * time.Second).String() }),
				detail.State,
				FormatOrNull(detail.LastError, func() string { return *detail.LastError }),
			)
		}

		return nil
	},
}

var utilWorkerPauseCmd = &cli.Command{
	Name:      "pause",
	Usage:     "Pause the specified sealing thread inside target worker",
	ArgsUsage: "<worker instance name or address> <thread index>",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 2 {
			return cli.ShowSubcommandHelp(cctx)
		}

		name := args.First()
		index, err := strconv.ParseUint(args.Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("parse thread index: %w", err)
		}

		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		dest, err := resolveWorkerDest(actx, a, name)
		if err != nil {
			return fmt.Errorf("resolve worker dest: %w", err)
		}

		wcli, wstop, err := workercli.Connect(context.Background(), fmt.Sprintf("http://%s/", dest))
		if err != nil {
			return fmt.Errorf("connect to %s: %w", dest, err)
		}

		defer wstop()

		ok, err := wcli.WorkerPause(index)
		if err != nil {
			return RPCCallError("WorkerPause", err)
		}

		Log.With("name", name, "dest", dest, "index", index).Infof("pause call done, ok = %v", ok)
		return nil
	},
}

var utilWorkerResumeCmd = &cli.Command{
	Name:      "resume",
	Usage:     "Resume the specified sealing thread inside target worker, with the given state if any",
	ArgsUsage: "<worker instance name or address> <thread index> [<next state>]",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 2 {
			return cli.ShowSubcommandHelp(cctx)
		}

		name := args.First()
		index, err := strconv.ParseUint(args.Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("parse thread index: %w", err)
		}

		var state *string
		if st := args.Get(2); st != "" {
			state = &st
		}

		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		dest, err := resolveWorkerDest(actx, a, name)
		if err != nil {
			return fmt.Errorf("resolve worker dest: %w", err)
		}

		wcli, wstop, err := workercli.Connect(context.Background(), fmt.Sprintf("http://%s/", dest))
		if err != nil {
			return fmt.Errorf("connect to %s: %w", dest, err)
		}

		defer wstop()

		ok, err := wcli.WorkerResume(index, state)
		if err != nil {
			return RPCCallError("WorkerPause", err)
		}

		Log.With("name", name, "dest", dest, "index", index, "state", state).Infof("resume call done, ok = %v", ok)
		return nil
	},
}

func resolveWorkerDest(ctx context.Context, a *APIClient, name string) (string, error) {
	var info *core.WorkerPingInfo
	var err error
	if a != nil {
		info, err = a.Damocles.WorkerGetPingInfo(ctx, name)
		if err != nil {
			return "", RPCCallError("WorkerGetPingInfo", err)
		}
	}

	if info != nil {
		return info.Info.Dest, nil
	}

	addr, err := net.ResolveTCPAddr("tcp", name)
	if err != nil {
		ip, err := net.ResolveIPAddr("", name)
		if err != nil {
			return "", fmt.Errorf("no instance found, and unable to parse %q as address", name)
		}

		addr = &net.TCPAddr{
			IP:   ip.IP,
			Zone: ip.Zone,
		}
	}

	if addr.Port == 0 {
		addr.Port = core.DefaultWorkerListenPort
	}

	return addr.String(), nil
}

var utilWdPostCmd = &cli.Command{
	Name:  "wdpost",
	Usage: "manager wdpost task when the task is handle by worker",
	Subcommands: []*cli.Command{
		utilWdPostListCmd,
		utilWdPostResetCmd,
	},
}

var utilWdPostListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all wdpost task",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "list all wdpost task, include the task that has been succeed",
		},
	},
	Action: func(cctx *cli.Context) error {
		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		var tasks []*core.WdPoStTask
		tasks, err = a.Damocles.WdPoStAllTasks(actx)
		if err != nil {
			return fmt.Errorf("get wdpost tasks: %w", err)
		}

		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		_, err = w.Write([]byte("ID\tMinerID\tWorker\tState\tCreateAt\tStartedAt\tHeartbeatAt\tFinishedAt\tError\n"))
		if err != nil {
			return err
		}
		for _, task := range tasks {

			state := "ReadyToRun"
			if task.StartedAt != 0 {
				state = "Running"
			}
			if task.FinishedAt != 0 {
				if task.ErrorReason != "" {
					state = "Failed"
				} else {
					state = "Succeed"
				}
			}

			if !cctx.Bool("all") && state == "Succeed" {
				continue
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				task.ID,
				task.Input.MinerID,
				task.WorkerName,
				state,
				time.Unix(int64(task.CreatedAt), 0),
				time.Unix(int64(task.StartedAt), 0),
				time.Unix(int64(task.HeartbeatAt), 0),
				time.Unix(int64(task.FinishedAt), 0),
				task.ErrorReason,
			)
		}

		w.Flush()
		return nil
	},
}

var utilWdPostResetCmd = &cli.Command{
	Name:      "reset",
	Usage:     "reset wdpost task",
	ArgsUsage: "<task id>",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 1 {
			return cli.ShowSubcommandHelp(cctx)
		}

		id := args.First()
		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		err = a.Damocles.WdPoStResetTask(actx, id)
		if err != nil {
			return fmt.Errorf("reset wdpost task: %w", err)
		}

		return nil
	},
}

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
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/strings"
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
		_, _ = fmt.Fprintln(tw, "Name\tDest\tVersion\tThreads\tEmpty\tPaused\tRunning\tWaiting\tErrors\tLastPing(with ! if expired)")
		for _, pinfo := range pinfos {
			lastPing := time.Since(time.Unix(pinfo.LastPing, 0))
			lastPingWarn := ""
			if lastPing > expiration {
				lastPingWarn = " (!)"
			}

			_, _ = fmt.Fprintf(
				tw, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%s%s\n",
				pinfo.Info.Name,
				pinfo.Info.Dest,
				pinfo.Info.Version,
				pinfo.Info.Summary.Threads,
				pinfo.Info.Summary.Empty,
				pinfo.Info.Summary.Paused,
				pinfo.Info.Summary.Running,
				pinfo.Info.Summary.Waiting,
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
		_, _ = fmt.Fprintln(tw, "Index\tLoc\tPlan\tJobID\tJobState\tJobStage\tThreadState\tLastErr")

		for _, detail := range details {
			_, _ = fmt.Fprintf(
				tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				detail.Index,
				detail.Location,
				detail.Plan,
				FormatOrNull(detail.JobID, func() string { return *detail.JobID }),
				detail.JobState,
				FormatOrNull(detail.JobStage, func() string { return *detail.JobStage }),
				detail.ThreadState,
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
	Usage: "manager wdpost jobs if the jobs is handle by worker",
	Subcommands: []*cli.Command{
		utilWdPostListCmd,
		utilWdPostResetCmd,
		utilWdPostRemoveCmd,
		utilWdPostRemoveAllCmd,
	},
}

var utilWdPostListCmd = &cli.Command{
	Name:  "list",
	Usage: "list all wdpost job",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "all",
			Usage: "list all wdpost job, include the job that has been succeed",
		},
		&cli.BoolFlag{
			Name:  "detail",
			Usage: "show more detailed information",
		},
	},
	Action: func(cctx *cli.Context) error {
		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		var jobs []core.WdPoStJobBrief
		jobs, err = a.Damocles.WdPoStAllJobs(actx)
		if err != nil {
			return fmt.Errorf("get wdpost jobs: %w", err)
		}

		detail := cctx.Bool("detail")

		w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
		if detail {
			_, err = w.Write([]byte("JobID\tPrefix\tMiner\tDDL\tPartitions\tWorker\tState\tTry\tCreateAt\tStartedAt\tHeartbeatAt\tFinishedAt\tUpdatedAt\tError\n"))
		} else {
			_, err = w.Write([]byte("JobID\tMinerID\tDDL\tPartitions\tWorker\tState\tTry\tCreateAt\tElapsed\tHeartbeat\tError\n"))
		}
		if err != nil {
			return err
		}
		formatDateTime := func(unix_secs uint64) string {
			if unix_secs == 0 {
				return "-"
			}
			return time.Unix(int64(unix_secs), 0).Format("01-02 15:04:05")
		}
		for _, job := range jobs {
			if !cctx.Bool("all") && job.Succeed() {
				continue
			}
			if detail {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					job.ID,
					job.State,
					job.Input.MinerID,
					job.DeadlineIdx,
					strings.Join(job.Partitions, ","),
					job.WorkerName,
					job.DisplayState(),
					job.TryNum,
					formatDateTime(job.CreatedAt),
					formatDateTime(job.StartedAt),
					formatDateTime(job.HeartbeatAt),
					formatDateTime(job.FinishedAt),
					formatDateTime(job.UpdatedAt),
					job.ErrorReason,
				)
			} else {
				var elapsed string
				var heartbeat string

				if job.StartedAt == 0 {
					elapsed = "-"
					heartbeat = "-"
				} else if job.FinishedAt == 0 {
					elapsed = time.Since(time.Unix(int64(job.StartedAt), 0)).Truncate(time.Second).String()
					heartbeat = time.Since(time.Unix(int64(job.HeartbeatAt), 0)).Truncate(time.Millisecond).String()
				} else {
					elapsed = fmt.Sprintf("%s(done)", time.Duration(job.FinishedAt-job.StartedAt)*time.Second)
					heartbeat = "-"
				}

				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n",
					job.ID,
					job.Input.MinerID,
					job.DeadlineIdx,
					strings.Join(job.Partitions, ","),
					job.WorkerName,
					job.DisplayState(),
					job.TryNum,
					formatDateTime(job.CreatedAt),
					elapsed,
					heartbeat,
					job.ErrorReason,
				)
			}
		}

		w.Flush()
		return nil
	},
}

var utilWdPostResetCmd = &cli.Command{
	Name:      "reset",
	Usage:     "reset the job status to allow new workers can pick it up",
	ArgsUsage: "<job id>...",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 1 {
			return cli.ShowSubcommandHelp(cctx)
		}

		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		for _, jobID := range args.Slice() {
			_, err = a.Damocles.WdPoStResetJob(actx, jobID)
			if err != nil {
				return fmt.Errorf("reset wdpost job: %w", err)
			}
		}

		return nil
	},
}

var utilWdPostRemoveCmd = &cli.Command{
	Name:      "remove",
	Usage:     "remove wdpost job",
	ArgsUsage: "<job id>...",
	Action: func(cctx *cli.Context) error {
		args := cctx.Args()
		if args.Len() < 1 {
			return cli.ShowSubcommandHelp(cctx)
		}

		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		for _, jobID := range args.Slice() {
			_, err = a.Damocles.WdPoStRemoveJob(actx, jobID)
			if err != nil {
				return fmt.Errorf("remove wdpost job: %w", err)
			}
		}
		return nil
	},
}

var utilWdPostRemoveAllCmd = &cli.Command{
	Name:  "remove-all",
	Usage: "remove all wdpost jobs",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "Actually perform the action",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			fmt.Println("Pass --really-do-it to actually execute this action")
			return nil
		}

		a, actx, stopper, err := extractAPI(cctx)
		if err != nil {
			return fmt.Errorf("get api: %w", err)
		}
		defer stopper()

		jobs, err := a.Damocles.WdPoStAllJobs(actx)
		if err != nil {
			return err
		}
		for _, job := range jobs {
			_, err = a.Damocles.WdPoStRemoveJob(actx, job.ID)
			if err != nil {
				return fmt.Errorf("remove wdpost job: %w", err)
			}
			fmt.Printf("wdpost job %s removed\n", job.ID)
		}
		return nil
	},
}

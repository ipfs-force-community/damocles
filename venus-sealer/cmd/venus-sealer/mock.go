package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/go-units"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sealer/sealing"
	"github.com/dtynn/venus-cluster/venus-sealer/util"
)

var mockCmd = &cli.Command{
	Name: "mock",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "miner",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "sector-size",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "listen",
			Value: ":1789",
		},
	},
	Action: func(cctx *cli.Context) error {
		sizeStr := cctx.String("sector-size")
		sectorSize, err := units.RAMInBytes(sizeStr)
		if err != nil {
			return fmt.Errorf("invalid sector-size string %s: %w", sizeStr, err)
		}

		proofType, err := util.SectorSize2SealProofType(uint64(sectorSize))
		if err != nil {
			return fmt.Errorf("get seal proof type: %w", err)
		}

		mockServer, err := sealing.NewMock(abi.ActorID(cctx.Uint64("miner")), proofType)
		if err != nil {
			return fmt.Errorf("construct mock server: %w", err)
		}

		httpHandler, err := buildRPCServer(mockServer)
		if err != nil {
			return fmt.Errorf("construct rpc server: %w", err)
		}

		httpServer := &http.Server{
			Addr:    cctx.String("listen"),
			Handler: httpHandler,
		}

		errCh := make(chan error, 1)

		go func() {
			log.Infof("trying to listen on %s", httpServer.Addr)

			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("http server error: %w", err)
			}
		}()

		shutdown := make(chan struct{}, 0)
		sigCh := make(chan os.Signal, 1)

		go func() {
			defer close(shutdown)

			select {
			case sig := <-sigCh:
				log.Warnf("signal %s captured", sig)

			case e := <-errCh:
				log.Errorf("error occured: %s", e)
			}

			if err := httpServer.Shutdown(context.Background()); err != nil {
				log.Errorf("shutdown http server: %s", err)
			} else {
				log.Info("http server shutdown")
			}

		}()

		signal.Notify(sigCh, syscall.SIGABRT, syscall.SIGTERM, syscall.SIGINT)

		<-shutdown
		_ = log.Sync()
		return nil
	},
}

func buildRPCServer(hdl interface{}, opts ...jsonrpc.ServerOption) (http.Handler, error) {
	server := jsonrpc.NewServer(opts...)
	server.Register("Venus", hdl)
	http.Handle("/rpc/v0", server)
	return http.DefaultServeMux, nil
}

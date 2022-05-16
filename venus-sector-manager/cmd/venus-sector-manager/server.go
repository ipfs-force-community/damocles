package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/dtynn/dix"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
)

func serveSealerAPI(ctx context.Context, stopper dix.StopFunc, node core.SealerAPI, addr string) error {
	mux, err := buildRPCServer(node)
	if err != nil {
		return fmt.Errorf("construct rpc server: %w", err)
	}

	// register piece store proxy

	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		log.Infof("trying to listen on %s", httpServer.Addr)

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("http server error: %w", err)
		}
	}()

	log.Info("daemon running")
	select {
	case <-ctx.Done():
		log.Warn("process signal captured")

	case e := <-errCh:
		log.Errorf("error occurred: %s", e)
	}

	log.Info("stop application")
	stopper(context.Background()) // nolint: errcheck

	log.Info("http server shutdown")
	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Errorf("shutdown http server: %s", err)
	}

	_ = log.Sync()
	return nil
}

func buildRPCServer(hdl interface{}, opts ...jsonrpc.ServerOption) (*http.ServeMux, error) {
	server := jsonrpc.NewServer(opts...)
	server.Register("Venus", hdl)
	http.Handle("/rpc/v0", server)
	return http.DefaultServeMux, nil
}

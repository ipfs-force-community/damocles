package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

func serveSealerAPI(node api.SealerAPI, addr string) error {
	httpHandler, err := buildRPCServer(node)
	if err != nil {
		return fmt.Errorf("construct rpc server: %w", err)
	}

	httpServer := &http.Server{
		Addr:    addr,
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
}

func buildRPCServer(hdl interface{}, opts ...jsonrpc.ServerOption) (http.Handler, error) {
	server := jsonrpc.NewServer(opts...)
	server.Register("Venus", hdl)
	http.Handle("/rpc/v0", server)
	return http.DefaultServeMux, nil
}

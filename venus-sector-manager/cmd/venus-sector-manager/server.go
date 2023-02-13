package main

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/dtynn/dix"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/metrics"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/metrics/proxy"
	vsmplugin "github.com/ipfs-force-community/venus-cluster/vsm-plugin"
)

func serveSealerAPI(ctx context.Context, stopper dix.StopFunc, node core.SealerAPI, addr string, plugins *vsmplugin.LoadedPlugins) error {
	mux, err := buildRPCServer(node, plugins)
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

func buildRPCServer(hdl interface{}, plugins *vsmplugin.LoadedPlugins, opts ...jsonrpc.ServerOption) (*http.ServeMux, error) {
	// use field
	opts = append(opts, jsonrpc.WithProxyBind(jsonrpc.PBField))
	hdl = proxy.MetricedSealerAPI(core.APINamespace, hdl)

	server := jsonrpc.NewServer(opts...)

	server.Register(core.APINamespace, hdl)

	if plugins != nil {
		_ = plugins.Foreach(vsmplugin.RegisterJsonRpc, func(plugin *vsmplugin.Plugin) error {
			m := vsmplugin.DeclareRegisterJsonRpcManifest(plugin.Manifest)
			namespace, hdl := m.Handler()
			log.Infof("register json rpc handler by plugin(%s). namespace: '%s'", plugin.Name, namespace)
			server.Register(namespace, proxy.MetricedSealerAPI(namespace, hdl))
			return nil
		})
	}

	http.Handle(fmt.Sprintf("/rpc/v%d", core.MajorVersion), server)

	// metrics
	http.Handle("/metrics", metrics.Exporter())
	return http.DefaultServeMux, nil
}

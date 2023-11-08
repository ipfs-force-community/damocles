module github.com/ipfs-force-community/damocles/damocles-manager

go 1.20

require (
	contrib.go.opencensus.io/exporter/prometheus v0.4.2
	github.com/BurntSushi/toml v1.2.1
	github.com/cespare/xxhash/v2 v2.2.0
	github.com/dgraph-io/badger/v2 v2.2007.4
	github.com/docker/go-units v0.5.0
	github.com/dtynn/dix v0.1.2
	github.com/etherlabsio/healthcheck/v2 v2.0.0
	github.com/fatih/color v1.13.0
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20220519234331-bfd1f5f9fe38
	github.com/filecoin-project/go-address v1.1.0
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-cbor-util v0.0.1
	github.com/filecoin-project/go-commp-utils v0.1.3
	github.com/filecoin-project/go-fil-commcid v0.1.0
	github.com/filecoin-project/go-jsonrpc v0.3.1
	github.com/filecoin-project/go-paramfetch v0.0.4
	github.com/filecoin-project/go-state-types v0.12.5
	github.com/filecoin-project/lotus v1.24.0-rc1
	github.com/filecoin-project/specs-storage v0.4.1
	github.com/filecoin-project/venus v1.14.0-rc4
	github.com/golang/mock v1.6.0
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/hashicorp/go-multierror v1.1.1
	github.com/ipfs-force-community/damocles/manager-plugin v0.0.0-20231020075910-8928215b9852
	github.com/ipfs-force-community/venus-cluster-assets v0.1.0
	github.com/ipfs/boxo v0.10.1
	github.com/ipfs/go-cid v0.4.1
	github.com/ipfs/go-datastore v0.6.0
	github.com/ipfs/go-ds-leveldb v0.5.0
	github.com/ipfs/go-ipld-cbor v0.0.6
	github.com/ipfs/go-log/v2 v2.5.1
	github.com/jbenet/go-random v0.0.0-20190219211222-123a90aedc0c
	github.com/libp2p/go-libp2p v0.27.9
	github.com/mitchellh/go-homedir v1.1.0
	github.com/mr-tron/base58 v1.2.0
	github.com/mroth/weightedrand v0.4.1
	github.com/multiformats/go-multiaddr v0.9.0
	github.com/multiformats/go-multihash v0.2.3
	github.com/shirou/gopsutil/v3 v3.22.5
	github.com/stretchr/testify v1.8.4
	github.com/strikesecurity/strikememongo v0.2.4
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	github.com/urfave/cli/v2 v2.25.5
	go.mongodb.org/mongo-driver v1.10.1
	go.opencensus.io v0.24.0
	go.uber.org/fx v1.19.3
	go.uber.org/zap v1.24.0
	golang.org/x/exp v0.0.0-20230321023759-10a507213a29
)

require (
	github.com/DataDog/zstd v1.4.5 // indirect
	github.com/GeertJohan/go.incremental v1.0.0 // indirect
	github.com/GeertJohan/go.rice v1.0.3 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/acobaugh/osrelease v0.0.0-20181218015638-a93a0a55a249 // indirect
	github.com/akavel/rsrc v0.8.0 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bluele/gcache v0.0.0-20190518031135-bc40bd653833 // indirect
	github.com/cespare/xxhash v1.1.0 // indirect
	github.com/cilium/ebpf v0.9.1 // indirect
	github.com/containerd/cgroups v1.1.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/crackcomm/go-gitignore v0.0.0-20170627025303-887ab5e44cc3 // indirect
	github.com/daaku/go.zipexe v1.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/detailyang/go-fallocate v0.0.0-20180908115635-432fa640bd2e // indirect
	github.com/dgraph-io/ristretto v0.1.1 // indirect
	github.com/dgryski/go-farm v0.0.0-20200201041132-a6ae2369ad13 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/elastic/go-sysinfo v1.7.0 // indirect
	github.com/elastic/go-windows v1.0.0 // indirect
	github.com/filecoin-project/dagstore v0.5.2 // indirect
	github.com/filecoin-project/go-amt-ipld/v2 v2.1.1-0.20201006184820-924ee87a1349 // indirect
	github.com/filecoin-project/go-amt-ipld/v3 v3.1.0 // indirect
	github.com/filecoin-project/go-amt-ipld/v4 v4.2.0 // indirect
	github.com/filecoin-project/go-crypto v0.0.1 // indirect
	github.com/filecoin-project/go-data-transfer/v2 v2.0.0-rc7 // indirect
	github.com/filecoin-project/go-fil-markets v1.28.3 // indirect
	github.com/filecoin-project/go-hamt-ipld v0.1.5 // indirect
	github.com/filecoin-project/go-hamt-ipld/v2 v2.0.0 // indirect
	github.com/filecoin-project/go-hamt-ipld/v3 v3.1.0 // indirect
	github.com/filecoin-project/go-padreader v0.0.1 // indirect
	github.com/filecoin-project/go-statemachine v1.0.3 // indirect
	github.com/filecoin-project/go-statestore v0.2.0 // indirect
	github.com/filecoin-project/go-storedcounter v0.1.0 // indirect
	github.com/filecoin-project/kubo-api-client v0.0.1 // indirect
	github.com/filecoin-project/pubsub v1.0.0 // indirect
	github.com/filecoin-project/specs-actors v0.9.15 // indirect
	github.com/filecoin-project/specs-actors/v2 v2.3.6 // indirect
	github.com/filecoin-project/specs-actors/v3 v3.1.2 // indirect
	github.com/filecoin-project/specs-actors/v4 v4.0.2 // indirect
	github.com/filecoin-project/specs-actors/v5 v5.0.6 // indirect
	github.com/filecoin-project/specs-actors/v6 v6.0.2 // indirect
	github.com/filecoin-project/specs-actors/v7 v7.0.1 // indirect
	github.com/filecoin-project/specs-actors/v8 v8.0.1 // indirect
	github.com/gbrlsnchs/jwt/v3 v3.0.1 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.1.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/hannahhoward/cbor-gen-for v0.0.0-20230214144701-5d17c9d5243c // indirect
	github.com/hannahhoward/go-pubsub v0.0.0-20200423002714-8d62886cc36e // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/golang-lru v0.6.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.2 // indirect
	github.com/icza/backscanner v0.0.0-20210726202459-ac2ffc679f94 // indirect
	github.com/ipfs/bbloom v0.0.4 // indirect
	github.com/ipfs/go-block-format v0.1.2 // indirect
	github.com/ipfs/go-blockservice v0.5.1 // indirect
	github.com/ipfs/go-fs-lock v0.0.7 // indirect
	github.com/ipfs/go-graphsync v0.14.6 // indirect
	github.com/ipfs/go-ipfs-blockstore v1.3.0 // indirect
	github.com/ipfs/go-ipfs-cmds v0.9.0 // indirect
	github.com/ipfs/go-ipfs-ds-help v1.1.0 // indirect
	github.com/ipfs/go-ipfs-exchange-interface v0.2.0 // indirect
	github.com/ipfs/go-ipfs-util v0.0.3 // indirect
	github.com/ipfs/go-ipld-format v0.5.0 // indirect
	github.com/ipfs/go-ipld-legacy v0.2.1 // indirect
	github.com/ipfs/go-log v1.0.5 // indirect
	github.com/ipfs/go-merkledag v0.11.0 // indirect
	github.com/ipfs/go-metrics-interface v0.0.1 // indirect
	github.com/ipfs/go-verifcid v0.0.2 // indirect
	github.com/ipld/go-car v0.6.1 // indirect
	github.com/ipld/go-car/v2 v2.10.1 // indirect
	github.com/ipld/go-codec-dagpb v1.6.0 // indirect
	github.com/ipld/go-ipld-prime v0.20.0 // indirect
	github.com/ipsn/go-secp256k1 v0.0.0-20180726113642-9d62b9f0bc52 // indirect
	github.com/jbenet/goprocess v0.1.4 // indirect
	github.com/jessevdk/go-flags v1.4.0 // indirect
	github.com/joeshaw/multierror v0.0.0-20140124173710-69b34d4ec901 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/kelseyhightower/envconfig v1.4.0 // indirect
	github.com/klauspost/compress v1.16.6 // indirect
	github.com/klauspost/cpuid/v2 v2.2.5 // indirect
	github.com/libp2p/go-buffer-pool v0.1.0 // indirect
	github.com/libp2p/go-flow-metrics v0.1.0 // indirect
	github.com/libp2p/go-libp2p-pubsub v0.9.3 // indirect
	github.com/libp2p/go-msgio v0.3.0 // indirect
	github.com/magefile/mage v1.11.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-runewidth v0.0.10 // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/miekg/dns v1.1.54 // indirect
	github.com/minio/blake2b-simd v0.0.0-20160723061019-3f5f724cb5b1 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/montanaflynn/stats v0.6.6 // indirect
	github.com/multiformats/go-base32 v0.1.0 // indirect
	github.com/multiformats/go-base36 v0.2.0 // indirect
	github.com/multiformats/go-multiaddr-dns v0.3.1 // indirect
	github.com/multiformats/go-multibase v0.2.0 // indirect
	github.com/multiformats/go-multicodec v0.9.0 // indirect
	github.com/multiformats/go-multistream v0.4.1 // indirect
	github.com/multiformats/go-varint v0.0.7 // indirect
	github.com/nkovacs/streamquote v1.0.0 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/petar/GoLLRB v0.0.0-20210522233825-ae3b015fd3e9 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/polydawn/refmt v0.89.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_golang v1.14.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/prometheus/statsd_exporter v0.23.0 // indirect
	github.com/puzpuzpuz/xsync/v2 v2.4.1 // indirect
	github.com/raulk/clock v1.1.0 // indirect
	github.com/rivo/uniseg v0.1.0 // indirect
	github.com/rs/cors v1.7.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil v2.18.12+incompatible // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/spf13/afero v1.8.2 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.1.0 // indirect
	github.com/whyrusleeping/bencher v0.0.0-20190829221104-bb6607aa8bba // indirect
	github.com/whyrusleeping/cbor v0.0.0-20171005072247-63513f603b11 // indirect
	github.com/whyrusleeping/cbor-gen v0.0.0-20230923211252-36a87e1ba72f // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.1 // indirect
	github.com/xdg-go/stringprep v1.0.3 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	github.com/youmark/pkcs8 v0.0.0-20181117223130-1be2e3e5546d // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.opentelemetry.io/otel v1.16.0 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	go.opentelemetry.io/otel/trace v1.16.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/dig v1.17.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go4.org v0.0.0-20230225012048-214862532bf5 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sync v0.2.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/tools v0.9.1 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/cheggaaa/pb.v1 v1.0.28 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v0.0.0-20181124034731-591f970eefbb // indirect
	lukechampine.com/blake3 v1.2.1 // indirect
)

replace (
	github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
	github.com/filecoin-project/go-jsonrpc => github.com/ipfs-force-community/go-jsonrpc v0.1.8
)

ARG RUST_TOOLCHAIN

FROM rust:${RUST_TOOLCHAIN}-bullseye AS build

RUN sed -i 's/deb.debian.org/mirrors.ustc.edu.cn/g' /etc/apt/sources.list
RUN apt-get update
RUN apt-get install -y \
    --no-install-recommends \
    tzdata \
    make \
    ca-certificates \
    build-essential \
    clang \
    ocl-icd-opencl-dev \
    ocl-icd-libopencl1 \
    libhwloc-dev \
    libssl-dev \
    pkg-config \
    libclang-dev

WORKDIR /build/
COPY . /build/

ARG GIT_FALLBACK="Unknown (no git or not git repo)"
ARG GIT_COMMIT=${GIT_FALLBACK}
ARG RUSTFLAGS=""

ENV CARGO_NET_GIT_FETCH_WITH_CLI=true

RUN cargo clean -q
RUN make build-all
RUN make /dependencies
RUN ldd /build/dist/bin/damocles-worker | grep "=>" | awk '{print $3}' | xargs -I {} cp {} /dependencies

FROM ubuntu:22.04

COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=build /etc/ssl/certs              /etc/ssl/certs
COPY --from=build /dependencies/              /lib/

RUN mkdir -p /etc/OpenCL/vendors \
 && echo "libnvidia-opencl.so.1" > /etc/OpenCL/vendors/nvidia.icd

COPY --from=build /build/target/release/damocles-worker /damocles-worker

EXPOSE 17890
ENTRYPOINT ["/damocles-worker"]

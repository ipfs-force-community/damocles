ARG RUST_TOOLCHAIN

FROM rust:${RUST_TOOLCHAIN}-buster AS build

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

RUN cargo clean -q
RUN make build-all

FROM ubuntu:20.04

COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=build /etc/ssl/certs              /etc/ssl/certs
COPY --from=build /lib/*/libdl.so.2           /lib/
COPY --from=build /lib/*/librt.so.1           /lib/
COPY --from=build /lib/*/libgcc_s.so.1        /lib/
COPY --from=build /lib/*/libutil.so.1         /lib/
COPY --from=build /usr/lib/*/libltdl.so.7     /lib/
COPY --from=build /usr/lib/*/libnuma.so.1     /lib/
COPY --from=build /usr/lib/*/libhwloc.so.5    /lib/
COPY --from=build /usr/lib/*/libOpenCL.so.1   /lib/
COPY --from=build /usr/lib/*/libssl.so.1.1    /lib/
COPY --from=build /usr/lib/*/libcrypto.so.1.1 /lib/

RUN mkdir -p /etc/OpenCL/vendors \
 && echo "libnvidia-opencl.so.1" > /etc/OpenCL/vendors/nvidia.icd

COPY --from=build /build/target/release/venus-worker /venus-worker

EXPOSE 17891
ENTRYPOINT ["/venus-worker"]

# bjarne-validator: Multi-stage Wolfi container for C/C++ validation
#
# Uses Wolfi (Chainguard) base image for:
# - Minimal attack surface
# - Zero known CVEs
# - SBOM included
# - glibc-based (required for sanitizers)
#
# Multi-stage build:
# - Stage 1 (builder): Build MSan-instrumented libc++
# - Stage 2 (runtime): Minimal image with only runtime dependencies
#
# Contains:
# - Clang 21 with full sanitizer support
# - clang-tidy for static analysis
# - lizard for complexity metrics
# - AddressSanitizer (ASAN)
# - UndefinedBehaviorSanitizer (UBSAN)
# - ThreadSanitizer (TSAN)
# - MemorySanitizer (MSan) with custom libc++
#
# Build: podman build -f docker/Dockerfile -t bjarne-validator:wolfi ./docker/
# Run:   podman run --rm -v $(pwd)/code:/src:ro bjarne-validator:wolfi clang++ --version

# ============================================================
# Stage 1: Builder - Build MSan-instrumented libc++
# ============================================================
FROM cgr.dev/chainguard/wolfi-base:latest AS builder

# Install build dependencies (includes python3 for CMake's LLVM build)
RUN apk update && apk add --no-cache \
    clang-21 \
    clang-21-extras \
    lld-21 \
    libLLVM-21 \
    compiler-rt-21 \
    libcxx1-21 \
    libcxx1-21-dev \
    libcxxabi1-21 \
    glibc-dev \
    linux-headers \
    cmake \
    samurai \
    git \
    python-3.13

# Build MSan-instrumented libc++ for MemorySanitizer support
# MSan requires ALL code (including stdlib) to be instrumented
RUN git clone --depth 1 --branch main https://github.com/llvm/llvm-project.git /tmp/llvm \
    && cmake -G Ninja -S /tmp/llvm/runtimes -B /tmp/llvm/build \
        -DCMAKE_C_COMPILER=/usr/bin/clang \
        -DCMAKE_CXX_COMPILER=/usr/bin/clang++ \
        -DLLVM_ENABLE_RUNTIMES="libcxx;libcxxabi;libunwind" \
        -DLLVM_USE_SANITIZER=MemoryWithOrigins \
        -DCMAKE_BUILD_TYPE=Release \
        -DLIBCXX_ENABLE_SHARED=OFF \
        -DLIBCXXABI_ENABLE_SHARED=OFF \
        -DLIBCXX_INCLUDE_TESTS=OFF \
        -DLIBCXX_INCLUDE_BENCHMARKS=OFF \
        -DLIBCXXABI_INCLUDE_TESTS=OFF \
        -DLIBUNWIND_INCLUDE_TESTS=OFF \
    && ninja -C /tmp/llvm/build cxx cxxabi \
    && mkdir -p /opt/msan \
    && cp -r /tmp/llvm/build/include /opt/msan/ \
    && cp -r /tmp/llvm/build/lib /opt/msan/ \
    && rm -rf /tmp/llvm

# ============================================================
# Stage 2: Runtime - Minimal image with runtime dependencies
# ============================================================
FROM cgr.dev/chainguard/wolfi-base:latest AS runtime

LABEL org.opencontainers.image.title="bjarne-validator"
LABEL org.opencontainers.image.description="C/C++ validation container with Clang 21 and sanitizers (Wolfi-based)"
LABEL org.opencontainers.image.source="https://github.com/3rg0n/bjarne"

# Install runtime dependencies only (no cmake, git, samurai needed)
RUN apk update && apk add --no-cache \
    clang-21 \
    clang-21-extras \
    lld-21 \
    libLLVM-21 \
    compiler-rt-21 \
    libcxx1-21 \
    libcxx1-21-dev \
    libcxxabi1-21 \
    glibc-dev \
    linux-headers \
    make \
    python-3.13 \
    py3.13-pip

# Create symlinks for tools not automatically linked by the package
RUN ln -sf /usr/lib/llvm-21/bin/lld /usr/bin/lld 2>/dev/null || true && \
    ln -sf /usr/lib/llvm-21/bin/llvm-symbolizer /usr/bin/llvm-symbolizer 2>/dev/null || true

# Install lizard for complexity metrics
RUN pip3 install --break-system-packages --no-cache-dir lizard

# Copy MSan-instrumented libc++ from builder stage
COPY --from=builder /opt/msan /opt/msan

# Set environment variables for sanitizers
ENV ASAN_SYMBOLIZER_PATH=/usr/bin/llvm-symbolizer
ENV ASAN_OPTIONS=symbolize=1:detect_leaks=1:print_stacktrace=1
ENV UBSAN_OPTIONS=print_stacktrace=1:halt_on_error=1
ENV TSAN_OPTIONS=second_deadlock_stack=1
ENV MSAN_OPTIONS=symbolize=1:print_stacktrace=1:halt_on_error=1
ENV MSAN_LIBCXX_PATH=/opt/msan

# Create working directory
WORKDIR /workspace
RUN mkdir -p /src && chmod 755 /src

# Default command: show version info
CMD ["clang++", "--version"]

# ============================================================
# Validation Commands (for reference)
# ============================================================
#
# 1. Static Analysis (clang-tidy):
#    clang-tidy /src/code.cpp -- -std=c++17 -Wall -Wextra
#
# 2. Compilation with warnings + hardening flags:
#    clang++ -std=c++17 -Wall -Wextra -Werror \
#      -fstack-protector-all -D_FORTIFY_SOURCE=2 \
#      -fPIE -pie -Wl,-z,relro -Wl,-z,now \
#      -o /tmp/test /src/code.cpp
#
#    Hardening flags:
#    - -fstack-protector-all: Stack buffer overflow detection
#    - -D_FORTIFY_SOURCE=2: Runtime buffer overflow checks
#    - -fPIE -pie: Position Independent Executable (ASLR)
#    - -Wl,-z,relro -Wl,-z,now: Full RELRO (GOT protection)
#
# 3. AddressSanitizer (memory errors):
#    clang++ -std=c++17 -fsanitize=address -fno-omit-frame-pointer -g -o /tmp/test /src/code.cpp
#    /tmp/test
#
# 4. UndefinedBehaviorSanitizer:
#    clang++ -std=c++17 -fsanitize=undefined -fno-omit-frame-pointer -g -o /tmp/test /src/code.cpp
#    /tmp/test
#
# 5. ThreadSanitizer (data races):
#    clang++ -std=c++17 -fsanitize=thread -fno-omit-frame-pointer -g -o /tmp/test /src/code.cpp
#    /tmp/test
#
# 6. MemorySanitizer (uninitialized reads) - requires MSan libc++:
#    clang++ -std=c++17 -fsanitize=memory -fsanitize-memory-track-origins \
#      -fno-omit-frame-pointer -g -stdlib=libc++ \
#      -nostdinc++ -isystem /opt/msan/include/c++/v1 \
#      -L/opt/msan/lib -Wl,-rpath,/opt/msan/lib \
#      -o /tmp/test /src/code.cpp
#    /tmp/test
#
# 7. Complexity check (lizard):
#    lizard -C 15 -L 100 /src/code.cpp
# ============================================================

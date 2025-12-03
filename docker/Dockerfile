# bjarne-validator: Validation container for C/C++ code
#
# Uses Wolfi (Chainguard) base image for:
# - Minimal attack surface
# - No CVEs
# - SBOM included
# - Designed for secure containerized workloads
#
# Contains:
# - Clang 18+ with full sanitizer support
# - clang-tidy for static analysis
# - AddressSanitizer (ASAN)
# - UndefinedBehaviorSanitizer (UBSAN)
# - ThreadSanitizer (TSAN)
# - MemorySanitizer (MSAN)
#
# Build: podman build -t bjarne-validator:dev ./docker/
# Run:   podman run --rm -v $(pwd)/code:/src:ro bjarne-validator:dev /src/test.cpp

# Wolfi base with glibc (required for sanitizers)
FROM cgr.dev/chainguard/wolfi-base:latest

LABEL org.opencontainers.image.title="bjarne-validator"
LABEL org.opencontainers.image.description="C/C++ validation container with Clang and sanitizers (Wolfi-based)"
LABEL org.opencontainers.image.source="https://github.com/ecopelan/bjarne"

# Install Clang and required tools from Wolfi packages
# Wolfi uses apk package manager
RUN apk update && apk add --no-cache \
    clang-18 \
    clang-18-dev \
    lld-18 \
    llvm-18 \
    compiler-rt \
    libcxx \
    libcxxabi \
    make \
    musl-dev \
    linux-headers

# Create symlinks for clang tools
RUN ln -sf /usr/bin/clang-18 /usr/bin/clang && \
    ln -sf /usr/bin/clang++-18 /usr/bin/clang++ && \
    ln -sf /usr/bin/clang-tidy-18 /usr/bin/clang-tidy 2>/dev/null || true && \
    ln -sf /usr/bin/lld-18 /usr/bin/lld

# Create non-root user for security (Wolfi runs as nonroot by default)
# Wolfi images are already rootless, but we ensure /src is accessible
RUN mkdir -p /src && chmod 755 /src

# Set working directory
WORKDIR /src

# Default command: show version info
CMD ["clang++", "--version"]

# ============================================================
# Validation Commands (for reference)
# ============================================================
#
# 1. Static Analysis (clang-tidy):
#    clang-tidy /src/code.cpp -- -std=c++17 -Wall -Wextra
#
# 2. Compilation with warnings:
#    clang++ -std=c++17 -Wall -Wextra -Werror -o /tmp/test /src/code.cpp
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
# 6. MemorySanitizer (uninitialized reads):
#    clang++ -std=c++17 -fsanitize=memory -fno-omit-frame-pointer -g -stdlib=libc++ -o /tmp/test /src/code.cpp
#    /tmp/test
#
# 7. Full hardening (release build verification):
#    clang++ -std=c++17 -O2 -Wall -Wextra -Werror \
#      -fstack-protector-all -D_FORTIFY_SOURCE=2 \
#      -fPIE -pie \
#      -Wl,-z,relro,-z,now -Wl,-z,noexecstack \
#      -o /tmp/test /src/code.cpp
# ============================================================

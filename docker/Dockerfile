# bjarne-validator: Wolfi container for C/C++ validation
#
# Uses Wolfi (Chainguard) base image for:
# - Minimal attack surface
# - Zero known CVEs
# - SBOM included
# - glibc-based (required for sanitizers)
#
# Contains:
# - Clang 21 with full sanitizer support
# - clang-tidy for static analysis
# - lizard for complexity metrics
# - AddressSanitizer (ASAN)
# - UndefinedBehaviorSanitizer (UBSAN)
# - ThreadSanitizer (TSAN)
# - MemorySanitizer (MSan) - detects uninitialized heap memory
#
# Build: podman build -f docker/Dockerfile -t bjarne-validator:wolfi ./docker/
# Run:   podman run --rm -v $(pwd)/code:/src:ro bjarne-validator:wolfi clang++ --version

FROM cgr.dev/chainguard/wolfi-base:latest

LABEL org.opencontainers.image.title="bjarne-validator"
LABEL org.opencontainers.image.description="C/C++ validation container with Clang 21 and sanitizers (Wolfi-based)"
LABEL org.opencontainers.image.source="https://github.com/3rg0n/bjarne"

# Install runtime dependencies
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

# Set environment variables for sanitizers
ENV ASAN_SYMBOLIZER_PATH=/usr/bin/llvm-symbolizer
ENV ASAN_OPTIONS=symbolize=1:detect_leaks=1:print_stacktrace=1
ENV UBSAN_OPTIONS=print_stacktrace=1:halt_on_error=1
ENV TSAN_OPTIONS=second_deadlock_stack=1
ENV MSAN_OPTIONS=symbolize=1:print_stacktrace=1:halt_on_error=1

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
# 6. MemorySanitizer (uninitialized heap reads):
#    clang++ -std=c++17 -fsanitize=memory -fsanitize-memory-track-origins \
#      -fno-omit-frame-pointer -g -O1 -o /tmp/test /src/code.cpp
#    MSAN_OPTIONS=halt_on_error=1 /tmp/test
#
#    Note: MSan detects uninitialized heap memory (malloc/new). For full stack
#    variable detection, a fully instrumented libc++ would be needed.
#
# 7. Complexity check (lizard):
#    lizard -C 15 -L 100 /src/code.cpp
# ============================================================

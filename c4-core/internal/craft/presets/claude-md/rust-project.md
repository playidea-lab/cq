---
name: rust-project
description: Rust 프로젝트용 CLAUDE.md — cargo, clippy, unsafe 정책, 에러 핸들링 컨벤션
---

# Project Instructions

## Build & Test

```bash
cargo build
cargo test
cargo clippy -- -D warnings
cargo fmt --check
```

## Code Style

- Error handling: `thiserror` for library errors, `anyhow` for application errors
- Avoid `unwrap()`/`expect()` in production code — propagate with `?`
- Prefer `impl Trait` for function arguments, named types for return
- Use `#[derive(Debug, Clone, PartialEq)]` where appropriate
- Lifetimes: explicit only when compiler cannot infer

## Unsafe Policy

- `unsafe` blocks require a `// SAFETY:` comment explaining the invariant
- No `unsafe` in library public API without strong justification
- Prefer safe abstractions (e.g., `bytes::Bytes` over raw pointers)

## Performance

- Minimize allocations in hot paths: prefer `&str` over `String`, `&[T]` over `Vec<T>`
- Use `Arc<T>` for shared ownership, `Rc<T>` only in single-threaded contexts
- Profile before optimizing: `cargo flamegraph` or `perf`

# CUSTOMIZE: 프로젝트 크레이트 구조, 주요 의존성 (tokio, axum, serde 등), MSRV 설정

## Project Structure

<!-- 크레이트 구조 설명 -->
<!-- 예: workspace 사용 여부, 주요 crate 목록 -->

## Key Dependencies

<!-- 주요 의존성과 버전 -->
<!-- 예: tokio = "1", serde = { version = "1", features = ["derive"] } -->

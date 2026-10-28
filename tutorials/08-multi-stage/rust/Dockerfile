# hello-rs 的 multi-stage Dockerfile（第 08 章，Rust 對照組）
#
#   docker build -t hello-rs:ch08 tutorials/08-multi-stage/rust
#   docker run --rm -p 8080:8080 hello-rs:ch08
#
# 注意 build context 是 rust/ 這個資料夾本身（不是 app/）。

# ── chef：裝好 cargo-chef 的共用底 ─────────────────────────
# builder 用 bookworm，runtime 用 distroless/cc-debian12：兩邊 glibc 同一代，才不會「build 得起來、跑不動」
FROM rust:1.95-slim-bookworm AS chef
RUN cargo install cargo-chef --locked
WORKDIR /app

# ── planner：從 Cargo.toml / Cargo.lock 算出「依賴清單」recipe.json ──
FROM chef AS planner
COPY . .
RUN cargo chef prepare --recipe-path recipe.json

# ── builder：先只編依賴，再編自己的程式 ─────────────────────
FROM chef AS builder
COPY --from=planner /app/recipe.json recipe.json
# 只要 recipe.json 沒變（依賴沒動），這層就吃快取 —— Rust 版的「先 COPY go.mod」（第 07 章）
RUN cargo chef cook --release --recipe-path recipe.json
COPY . .
RUN cargo build --release --bin hello-rs

# ── runtime：只有 glibc + CA 憑證，沒有 cargo、沒有原始碼 ──────
FROM gcr.io/distroless/cc-debian12 AS runtime
COPY --from=builder /app/target/release/hello-rs /hello-rs
EXPOSE 8080
ENTRYPOINT ["/hello-rs"]

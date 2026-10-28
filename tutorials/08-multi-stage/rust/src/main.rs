//! hello-rs —— 第 08、09、23 章用來跟 shorty（Go）對照的最小 Rust 服務。
//!
//!   cargo run
//!   curl localhost:8080/

use axum::{Router, routing::get};

#[tokio::main]
async fn main() {
    let app = Router::new().route("/", get(|| async { "hello from Rust (axum)\n" }));

    // 要聽 0.0.0.0：container 裡聽 127.0.0.1 的話，-p 轉進來的連線會連不到（第 12 章）
    let listener = tokio::net::TcpListener::bind("0.0.0.0:8080").await.unwrap();
    println!("listening on {}", listener.local_addr().unwrap());

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await
        .unwrap();
}

// 收到 Ctrl+C（SIGINT）或 docker stop 的 SIGTERM 就收工（第 16 章）
async fn shutdown_signal() {
    let ctrl_c = async { tokio::signal::ctrl_c().await.unwrap() };

    #[cfg(unix)]
    let terminate = async {
        tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .unwrap()
            .recv()
            .await;
    };
    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}

mod config;
mod relay;

use anyhow::Result;

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize logging
    tracing_subscriber::fmt::init();

    tracing::info!("nautilus relay starting");

    let cfg = match config::Config::load() {
        Ok(c) => c,
        Err(e) => {
            tracing::error!(error = ?e, "initialization failed: configuration error");
            return Err(e);
        }
    };

    // Ensure base directory exists
    if let Err(e) = std::fs::create_dir_all(&cfg.base_dir) {
        tracing::error!(error = ?e, path = ?cfg.base_dir, "failed to create base directory");
        return Err(e.into());
    }

    let r = relay::Relay::new(cfg);
    if let Err(e) = r.run().await {
        tracing::error!(error = ?e, "runtime fatal error");
        return Err(e);
    }

    Ok(())
}

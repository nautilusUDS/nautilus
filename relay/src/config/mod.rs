use anyhow::Result;
use std::env;
use std::path::PathBuf;

#[derive(Debug, Clone)]
pub struct Config {
    pub service_name: String,
    pub target_addr: String,
    pub socket_name: String,
    pub base_dir: PathBuf,
    pub max_connections: usize,
}

impl Config {
    pub fn load() -> Result<Self> {
        tracing::debug!("loading configuration from environment variables");

        let service_name = env::var("NAUTILUS_SERVICE_NAME")
            .or_else(|_| env::var("NAUTILUS_SERVICE_ID"))
            .map_err(|_| {
                tracing::error!(var = "NAUTILUS_SERVICE_NAME", "missing required environment variable");
                anyhow::anyhow!("NAUTILUS_SERVICE_NAME is required")
            })?;

        let target_addr = env::var("NAUTILUS_TARGET_ADDR").map_err(|_| {
            tracing::error!(var = "NAUTILUS_TARGET_ADDR", "missing required environment variable");
            anyhow::anyhow!("NAUTILUS_TARGET_ADDR is required")
        })?;

        let socket_name = env::var("NAUTILUS_SOCKET_NAME").map_err(|_| {
            tracing::error!(var = "NAUTILUS_SOCKET_NAME", "missing required environment variable");
            anyhow::anyhow!("NAUTILUS_SOCKET_NAME is required")
        })?;

        let base_dir = env::var("NAUTILUS_SERVICES_DIR")
            .unwrap_or_else(|_| "/var/run/nautilus/services".to_string())
            .into();

        let max_connections = env::var("NAUTILUS_MAX_CONNS")
            .unwrap_or_else(|_| "1024".to_string())
            .parse()
            .unwrap_or(1024);

        let cfg = Self {
            service_name,
            target_addr,
            socket_name,
            base_dir,
            max_connections,
        };

        tracing::info!(
            service = %cfg.service_name,
            target = %cfg.target_addr,
            socket = ?cfg.socket_path(),
            max_conns = cfg.max_connections,
            "configuration loaded"
        );

        Ok(cfg)
    }

    pub fn socket_path(&self) -> PathBuf {
        self.base_dir
            .join(&self.service_name)
            .join(&self.socket_name)
    }

    pub fn service_dir(&self) -> PathBuf {
        self.base_dir.join(&self.service_name)
    }
}

use crate::config::Config;
#[cfg(unix)]
use anyhow::Context;
use anyhow::Result;
#[cfg(unix)]
use std::fs;
use std::sync::Arc;
use std::time::Duration;
#[cfg(unix)]
use tokio::io::copy_bidirectional;
use tokio::net::TcpStream;
#[cfg(unix)]
use tokio::net::UnixListener;
use tokio::sync::{watch, Semaphore};
use tokio::time::interval;
#[cfg(unix)]
use tracing::error;
use tracing::{debug, info, warn};

pub struct Relay {
    cfg: Arc<Config>,
    pool: Arc<Semaphore>,
}

impl Relay {
    pub fn new(cfg: Config) -> Self {
        let max_conn = cfg.max_connections;
        Self {
            cfg: Arc::new(cfg),
            pool: Arc::new(Semaphore::new(max_conn)),
        }
    }

    pub async fn run(&self) -> Result<()> {
        let mut check_interval = interval(Duration::from_secs(2));
        let mut active_handle: Option<tokio::task::JoinHandle<()>> = None;
        let (stop_tx, _stop_rx) = watch::channel(());

        info!(
            target = %self.cfg.target_addr,
            max_conns = self.cfg.max_connections,
            "relay loop initialized"
        );

        loop {
            check_interval.tick().await;

            let is_alive = self.probe().await;
            debug!(target = %self.cfg.target_addr, alive = is_alive, "health probe result");

            if is_alive && active_handle.is_none() {
                info!(target = %self.cfg.target_addr, "target online, starting listener");
                let cfg = self.cfg.clone();
                let pool = self.pool.clone();
                let stop_rx = _stop_rx.clone();

                active_handle = Some(tokio::spawn(async move {
                    #[cfg(unix)]
                    if let Err(e) = run_uds_listener(cfg, pool, stop_rx).await {
                        error!(error = ?e, "uds listener failure");
                    }
                    #[cfg(not(unix))]
                    {
                        let _ = (cfg, pool, stop_rx);
                        warn!("uds listener is only supported on unix systems");
                        tokio::time::sleep(Duration::from_secs(3600)).await;
                    }
                }));
            } else if !is_alive && active_handle.is_some() {
                warn!(target = %self.cfg.target_addr, "target offline, stopping listener");
                if let Some(handle) = active_handle.take() {
                    let _ = stop_tx.send(());
                    handle.abort();
                    debug!("listener task terminated");
                }
            }
        }
    }

    async fn probe(&self) -> bool {
        match tokio::time::timeout(
            Duration::from_secs(1),
            TcpStream::connect(&self.cfg.target_addr),
        )
        .await
        {
            Ok(Ok(_)) => true,
            Ok(Err(e)) => {
                debug!(target = %self.cfg.target_addr, error = %e, "probe connection failed");
                false
            }
            Err(_) => {
                debug!(target = %self.cfg.target_addr, "probe timed out");
                false
            }
        }
    }
}

#[cfg(unix)]
async fn run_uds_listener(
    cfg: Arc<Config>,
    pool: Arc<Semaphore>,
    mut stop_rx: watch::Receiver<()>,
) -> Result<()> {
    let socket_path = cfg.socket_path();
    let service_dir = cfg.service_dir();
    let temp_socket_path = socket_path.with_extension("tmp");

    debug!(path = ?socket_path, "binding unix domain socket");

    // Ensure directory exists
    if !service_dir.exists() {
        info!(dir = ?service_dir, "creating service directory");
        fs::create_dir_all(&service_dir)?;
    }

    // Cleanup old sockets
    let _ = fs::remove_file(&temp_socket_path);
    let _ = fs::remove_file(&socket_path);

    let listener = UnixListener::bind(&temp_socket_path).context("Failed to bind UDS")?;

    // Set permissions (equivalent to chmod 0666)
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&temp_socket_path, fs::Permissions::from_mode(0o666))?;
    }

    fs::rename(&temp_socket_path, &socket_path).context("Failed to rename UDS")?;

    info!(path = ?socket_path, "uds listener active");

    struct CleanupOnDrop {
        path: std::path::PathBuf,
    }
    impl Drop for CleanupOnDrop {
        fn drop(&mut self) {
            info!(path = ?self.path, "cleaning up socket file");
            let _ = fs::remove_file(&self.path);
        }
    }
    let _cleanup = CleanupOnDrop {
        path: socket_path.clone(),
    };

    loop {
        tokio::select! {
            accept_res = listener.accept() => {
                match accept_res {
                    Ok((mut uds_stream, addr)) => {
                        let pool = pool.clone();
                        let target_addr = cfg.target_addr.clone();

                        // Acquire permit from connection pool
                        // If pool is full, this will wait (queue) until a spot opens
                        let permit = match pool.clone().acquire_owned().await {
                            Ok(p) => p,
                            Err(_) => {
                                error!("connection pool semaphore exhausted");
                                break;
                            }
                        };

                        debug!(
                            client = ?addr,
                            active_conns = cfg.max_connections - pool.available_permits(),
                            "connection accepted"
                        );

                        tokio::spawn(async move {
                            // Move permit into spawn to keep it alive for the duration of the connection
                            let _permit = permit;

                            match TcpStream::connect(&target_addr).await {
                                Ok(mut tcp_stream) => {
                                    debug!(target = %target_addr, "relaying stream");
                                    if let Err(e) = copy_bidirectional(&mut uds_stream, &mut tcp_stream).await {
                                        debug!(error = ?e, "connection closed");
                                    }
                                }
                                Err(e) => error!(target = %target_addr, error = ?e, "upstream connection failed"),
                            }
                            // Permit is automatically dropped here, returning to pool
                        });
                    }
                    Err(e) => {
                        error!(error = ?e, "uds accept failure");
                        break;
                    }
                }
            }
            _ = stop_rx.changed() => {
                info!("shutdown signal received, stopping uds listener");
                break;
            }
        }
    }

    Ok(())
}

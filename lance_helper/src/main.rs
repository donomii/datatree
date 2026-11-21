use std::io::{BufWriter, Write};
use std::path::PathBuf;
use std::sync::Arc;

use arrow_json::writer::LineDelimitedWriter;
use futures::TryStreamExt;
use lance_core::cache::LanceCache;
use lance_encoding::decoder::{DecoderPlugins, FilterExpression};
use lance_file::reader::{FileReader, FileReaderOptions};
use lance_io::ReadBatchParams;
use lance_io::object_store::ObjectStore;
use lance_io::scheduler::{ScanScheduler, SchedulerConfig};
use lance_io::utils::CachedFileSize;
use object_store::path::Path;

fn main() {
    match run() {
        Ok(()) => {}
        Err(error) => {
            eprintln!("cannot read Lance row values: {error}");
            std::process::exit(1);
        }
    }
}

fn run() -> Result<(), Box<dyn std::error::Error>> {
    let source = std::env::args_os()
        .nth(1)
        .ok_or("expected one Lance file path")?;
    let source = PathBuf::from(source);
    let runtime = tokio::runtime::Runtime::new()?;
    runtime.block_on(read_rows(source))
}

async fn read_rows(source: PathBuf) -> Result<(), Box<dyn std::error::Error>> {
    let store = ObjectStore::local();
    let scheduler_config = SchedulerConfig::max_bandwidth(&store);
    let scheduler = ScanScheduler::new(Arc::new(store), scheduler_config);
    let path = Path::from_filesystem_path(&source)?;
    let file = scheduler
        .open_file(&path, &CachedFileSize::unknown())
        .await?;
    let reader = FileReader::try_open(
        file,
        None,
        Arc::new(DecoderPlugins::default()),
        &LanceCache::no_cache(),
        FileReaderOptions::default(),
    )
    .await?;
    let mut batches = reader
        .read_stream(
            ReadBatchParams::RangeFull,
            1024,
            16,
            FilterExpression::no_filter(),
        )
        .await?;
    let output = BufWriter::new(std::io::stdout().lock());
    let mut writer = LineDelimitedWriter::new(output);
    while let Some(batch) = batches.try_next().await? {
        writer.write(&batch)?;
    }
    writer.finish()?;
    writer.into_inner().flush()?;
    Ok(())
}

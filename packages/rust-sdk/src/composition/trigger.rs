use super::wait::{WaitForRunOptions, wait_for_run};
use crate::errors::StraitError;
use std::future::Future;

pub async fn trigger_and_wait<
    TInput,
    TRun,
    TriggerFut,
    GetRunFut,
    TriggerFn,
    GetRunFn,
    GetIdFn,
    GetStatusFn,
>(
    input: TInput,
    trigger_fn: TriggerFn,
    get_run: GetRunFn,
    get_id: GetIdFn,
    get_status: GetStatusFn,
    opts: Option<&WaitForRunOptions>,
) -> Result<TRun, StraitError>
where
    TriggerFn: FnOnce(TInput) -> TriggerFut,
    TriggerFut: Future<Output = Result<TRun, StraitError>>,
    GetRunFn: Fn(&str) -> GetRunFut,
    GetRunFut: Future<Output = Result<TRun, StraitError>>,
    GetIdFn: FnOnce(&TRun) -> &str,
    GetStatusFn: Fn(&TRun) -> &str,
{
    let result = trigger_fn(input).await?;
    let run_id = get_id(&result).to_string();
    wait_for_run(&run_id, get_run, get_status, opts).await
}

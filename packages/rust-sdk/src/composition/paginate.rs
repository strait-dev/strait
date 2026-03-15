use crate::errors::StraitError;
use std::future::Future;

#[derive(Debug, Clone, Default)]
pub struct PaginatedQuery {
    pub cursor: Option<String>,
    pub limit: Option<u32>,
}

#[derive(Debug, Clone)]
pub struct PaginatedResponse<T> {
    pub data: Vec<T>,
    pub next_cursor: Option<String>,
    pub has_more: bool,
}

#[derive(Debug, Clone)]
pub struct PaginateOptions {
    pub limit: u32,
}

impl Default for PaginateOptions {
    fn default() -> Self {
        Self { limit: 50 }
    }
}

pub async fn collect_all<T, F, Fut>(
    list_fn: F,
    opts: Option<&PaginateOptions>,
) -> Result<Vec<T>, StraitError>
where
    F: Fn(PaginatedQuery) -> Fut,
    Fut: Future<Output = Result<PaginatedResponse<T>, StraitError>>,
{
    let default_opts = PaginateOptions::default();
    let opts = opts.unwrap_or(&default_opts);
    let mut all_items = Vec::new();
    let mut cursor = None;

    loop {
        let query = PaginatedQuery {
            cursor: cursor.clone(),
            limit: Some(opts.limit),
        };
        let response = list_fn(query).await?;
        all_items.extend(response.data);

        if !response.has_more || response.next_cursor.is_none() {
            break;
        }
        cursor = response.next_cursor;
    }

    Ok(all_items)
}

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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_paginated_query_default() {
        let q = PaginatedQuery::default();
        assert!(q.cursor.is_none());
        assert!(q.limit.is_none());
    }

    #[test]
    fn test_paginate_options_default() {
        let opts = PaginateOptions::default();
        assert_eq!(opts.limit, 50);
    }

    #[test]
    fn test_paginated_response_creation() {
        let resp = PaginatedResponse {
            data: vec![1, 2, 3],
            next_cursor: Some("cursor-abc".to_string()),
            has_more: true,
        };
        assert_eq!(resp.data.len(), 3);
        assert_eq!(resp.next_cursor, Some("cursor-abc".to_string()));
        assert!(resp.has_more);
    }

    #[test]
    fn test_paginated_response_no_more() {
        let resp: PaginatedResponse<i32> = PaginatedResponse {
            data: vec![1],
            next_cursor: None,
            has_more: false,
        };
        assert!(!resp.has_more);
        assert!(resp.next_cursor.is_none());
    }

    #[tokio::test]
    async fn test_collect_all_single_page() {
        let result = collect_all(
            |_q| async {
                Ok::<_, StraitError>(PaginatedResponse {
                    data: vec![1, 2, 3],
                    next_cursor: None,
                    has_more: false,
                })
            },
            None,
        )
        .await;
        assert_eq!(result.unwrap(), vec![1, 2, 3]);
    }

    #[tokio::test]
    async fn test_collect_all_multiple_pages() {
        let call_count = std::sync::Arc::new(std::sync::atomic::AtomicU32::new(0));
        let cc = call_count.clone();
        let result = collect_all(
            move |_q| {
                let cc = cc.clone();
                async move {
                    let n = cc.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                    match n {
                        0 => Ok::<_, StraitError>(PaginatedResponse {
                            data: vec![1, 2],
                            next_cursor: Some("c1".to_string()),
                            has_more: true,
                        }),
                        _ => Ok(PaginatedResponse {
                            data: vec![3],
                            next_cursor: None,
                            has_more: false,
                        }),
                    }
                }
            },
            None,
        )
        .await;
        assert_eq!(result.unwrap(), vec![1, 2, 3]);
    }

    #[tokio::test]
    async fn test_collect_all_empty() {
        let result = collect_all(
            |_q| async {
                Ok::<_, StraitError>(PaginatedResponse::<i32> {
                    data: vec![],
                    next_cursor: None,
                    has_more: false,
                })
            },
            None,
        )
        .await;
        assert_eq!(result.unwrap(), Vec::<i32>::new());
    }

    #[tokio::test]
    async fn test_collect_all_error() {
        let result = collect_all::<i32, _, _>(
            |_q| async {
                Err(StraitError::Transport {
                    message: "fail".to_string(),
                    cause: None,
                })
            },
            None,
        )
        .await;
        assert!(result.is_err());
    }
}

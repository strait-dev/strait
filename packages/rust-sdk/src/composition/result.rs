use std::fmt;

#[derive(Debug)]
pub enum StraitResult<T> {
    Ok(T),
    Err(Box<dyn std::error::Error + Send + Sync>),
}

impl<T> StraitResult<T> {
    pub fn is_ok(&self) -> bool {
        matches!(self, StraitResult::Ok(_))
    }

    pub fn is_err(&self) -> bool {
        matches!(self, StraitResult::Err(_))
    }

    pub fn unwrap(self) -> T {
        match self {
            StraitResult::Ok(v) => v,
            StraitResult::Err(e) => panic!("called unwrap on an Err: {}", e),
        }
    }

    pub fn unwrap_err(self) -> Box<dyn std::error::Error + Send + Sync> {
        match self {
            StraitResult::Ok(_) => panic!("called unwrap_err on an Ok"),
            StraitResult::Err(e) => e,
        }
    }

    pub fn match_result<R>(self, on_ok: impl FnOnce(T) -> R, on_err: impl FnOnce(Box<dyn std::error::Error + Send + Sync>) -> R) -> R {
        match self {
            StraitResult::Ok(v) => on_ok(v),
            StraitResult::Err(e) => on_err(e),
        }
    }
}

impl<T: fmt::Display> fmt::Display for StraitResult<T> {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            StraitResult::Ok(v) => write!(f, "Ok({})", v),
            StraitResult::Err(e) => write!(f, "Err({})", e),
        }
    }
}

pub fn from_fn<T, E: std::error::Error + Send + Sync + 'static>(f: impl FnOnce() -> Result<T, E>) -> StraitResult<T> {
    match f() {
        Ok(v) => StraitResult::Ok(v),
        Err(e) => StraitResult::Err(Box::new(e)),
    }
}

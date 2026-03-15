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

#[cfg(test)]
mod tests {
    use super::*;
    use std::io;

    #[test]
    fn test_ok_is_ok() {
        let r: StraitResult<i32> = StraitResult::Ok(42);
        assert!(r.is_ok());
    }

    #[test]
    fn test_ok_is_not_err() {
        let r: StraitResult<i32> = StraitResult::Ok(42);
        assert!(!r.is_err());
    }

    #[test]
    fn test_err_is_err() {
        let r: StraitResult<i32> = StraitResult::Err(Box::new(io::Error::new(io::ErrorKind::Other, "fail")));
        assert!(r.is_err());
    }

    #[test]
    fn test_err_is_not_ok() {
        let r: StraitResult<i32> = StraitResult::Err(Box::new(io::Error::new(io::ErrorKind::Other, "fail")));
        assert!(!r.is_ok());
    }

    #[test]
    fn test_unwrap_ok() {
        let r: StraitResult<i32> = StraitResult::Ok(42);
        assert_eq!(r.unwrap(), 42);
    }

    #[test]
    #[should_panic(expected = "called unwrap on an Err")]
    fn test_unwrap_err_panics() {
        let r: StraitResult<i32> = StraitResult::Err(Box::new(io::Error::new(io::ErrorKind::Other, "fail")));
        r.unwrap();
    }

    #[test]
    fn test_unwrap_err() {
        let r: StraitResult<i32> = StraitResult::Err(Box::new(io::Error::new(io::ErrorKind::Other, "fail")));
        let e = r.unwrap_err();
        assert_eq!(format!("{}", e), "fail");
    }

    #[test]
    #[should_panic(expected = "called unwrap_err on an Ok")]
    fn test_unwrap_err_on_ok_panics() {
        let r: StraitResult<i32> = StraitResult::Ok(42);
        r.unwrap_err();
    }

    #[test]
    fn test_match_result_ok() {
        let r: StraitResult<i32> = StraitResult::Ok(42);
        let v = r.match_result(|v| v * 2, |_| 0);
        assert_eq!(v, 84);
    }

    #[test]
    fn test_match_result_err() {
        let r: StraitResult<i32> = StraitResult::Err(Box::new(io::Error::new(io::ErrorKind::Other, "fail")));
        let v = r.match_result(|v| v * 2, |_| -1);
        assert_eq!(v, -1);
    }

    #[test]
    fn test_display_ok() {
        let r: StraitResult<i32> = StraitResult::Ok(42);
        assert_eq!(format!("{}", r), "Ok(42)");
    }

    #[test]
    fn test_display_err() {
        let r: StraitResult<i32> = StraitResult::Err(Box::new(io::Error::new(io::ErrorKind::Other, "fail")));
        assert_eq!(format!("{}", r), "Err(fail)");
    }

    #[test]
    fn test_from_fn_ok() {
        let r = from_fn(|| -> Result<i32, io::Error> { Ok(42) });
        assert!(r.is_ok());
        assert_eq!(r.unwrap(), 42);
    }

    #[test]
    fn test_from_fn_err() {
        let r = from_fn(|| -> Result<i32, io::Error> { Err(io::Error::new(io::ErrorKind::Other, "fail")) });
        assert!(r.is_err());
    }

    #[test]
    fn test_from_fn_err_message() {
        let r = from_fn(|| -> Result<i32, io::Error> { Err(io::Error::new(io::ErrorKind::NotFound, "not found")) });
        let e = r.unwrap_err();
        assert_eq!(format!("{}", e), "not found");
    }

    #[test]
    fn test_unwrap_string() {
        let r: StraitResult<String> = StraitResult::Ok("hello".to_string());
        assert_eq!(r.unwrap(), "hello");
    }
}

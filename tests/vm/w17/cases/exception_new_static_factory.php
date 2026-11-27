<?php
// exception_new_static_factory: static factory methods using new static and new self in an exception class hierarchy
class AppError extends Exception {
    public static function make($msg) {
        return new static($msg, 100);
    }

    public static function fallback() {
        return new self("fallback self");
    }
}

class DbError extends AppError {
    public static function connect() {
        return static::make("connect failed");
    }

    public static function viaSelf() {
        return self::fallback();
    }
}

class QueryError extends DbError {}

$a = AppError::make("app");
echo "app_class: ", $a::class, "\n";
echo "app_code: ", $a->getCode(), "\n";
echo "app_line: ", $a->getLine(), "\n";

$b = DbError::connect();
echo "db_class: ", $b::class, "\n";
echo "db_message: ", $b->getMessage(), "\n";
echo "db_code: ", $b->getCode(), "\n";
echo "db_line: ", $b->getLine(), "\n";

$c = QueryError::make("query");
echo "query_class: ", $c::class, "\n";
echo "query_is_app: ", ($c instanceof AppError ? 1 : 0), "\n";

$d = DbError::viaSelf();
echo "self_class_from_db: ", $d::class, "\n";
echo "self_message: ", $d->getMessage(), "\n";
echo "self_line: ", $d->getLine(), "\n";

$e = QueryError::fallback();
echo "fallback_class_from_query: ", $e::class, "\n";
echo "lines_differ_app_db: ", ($a->getLine() == $b->getLine() ? 0 : 1), "\n";
echo "fallback_is_db: ", ($e instanceof DbError ? 1 : 0), "\n";

<?php
// exception_basic_getters: getters of Exception and Error built with message, code and previous
$e = new Exception("m", 5);
echo "exc_message: ", $e->getMessage(), "\n";
echo "exc_code: ", $e->getCode(), "\n";
echo "exc_previous_is_null: ", ($e->getPrevious() === null ? 1 : 0), "\n";
echo "exc_line: ", $e->getLine(), "\n";
echo "exc_file: ", $e->getFile(), "\n";
echo "exc_class: ", $e::class, "\n";

$r = new Error("e", 7);
echo "err_message: ", $r->getMessage(), "\n";
echo "err_code: ", $r->getCode(), "\n";
echo "err_previous_is_null: ", ($r->getPrevious() === null ? 1 : 0), "\n";
echo "err_line: ", $r->getLine(), "\n";
echo "err_file: ", $r->getFile(), "\n";
echo "err_class: ", $r::class, "\n";

$p = new Exception("inner");
$q = new Exception("outer", 9, $p);
$qp = $q->getPrevious();
echo "outer_previous_message: ", $qp->getMessage(), "\n";
echo "outer_previous_is_p: ", ($qp === $p ? 1 : 0), "\n";
echo "outer_code: ", $q->getCode(), "\n";
echo "top_level_trace_empty: ", ($e->getTrace() === [] ? 1 : 0), "\n";

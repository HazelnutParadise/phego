<?php
// exception_default_values: no-argument and message-only construction of Exception and Error
$e = new Exception();
echo "exc_message: [", $e->getMessage(), "]\n";
echo "exc_code: ", $e->getCode(), "\n";
echo "exc_previous_null: ", ($e->getPrevious() === null ? 1 : 0), "\n";
echo "exc_class: ", $e::class, "\n";
echo "exc_line: ", $e->getLine(), "\n";
echo "exc_file: ", $e->getFile(), "\n";

$r = new Error();
echo "err_message: [", $r->getMessage(), "]\n";
echo "err_code: ", $r->getCode(), "\n";
echo "err_previous_null: ", ($r->getPrevious() === null ? 1 : 0), "\n";
echo "err_class: ", $r::class, "\n";
echo "err_line: ", $r->getLine(), "\n";

$m = new Exception("only message");
echo "only_message: ", $m->getMessage(), "\n";
echo "only_message_code: ", $m->getCode(), "\n";
echo "only_message_previous_null: ", ($m->getPrevious() === null ? 1 : 0), "\n";

$n = new Error("err only");
echo "err_only_message: ", $n->getMessage(), "\n";
echo "err_only_code: ", $n->getCode(), "\n";

$t = new Exception("", 0, null);
echo "explicit_null_previous: ", ($t->getPrevious() === null ? 1 : 0), "\n";
echo "explicit_empty_message: [", $t->getMessage(), "]\n";
echo "empty_string_form: ", (string)$e, "\n";

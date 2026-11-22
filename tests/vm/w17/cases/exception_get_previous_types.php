<?php
// exception_get_previous_types: previous exceptions of different class families and two-level getPrevious chains
$err = new Error("engine");
$ex = new Exception("user", 0, $err);
$p1 = $ex->getPrevious();
echo "exc_over_error_prev: ", $p1::class, "\n";
echo "exc_over_error_prev_msg: ", $p1->getMessage(), "\n";
echo "prev_is_error: ", ($p1 instanceof Error ? 1 : 0), "\n";
echo "prev_is_same: ", ($p1 === $err ? 1 : 0), "\n";

$e2 = new Error("outer error", 0, new TypeError("type inner"));
$e2p = $e2->getPrevious();
echo "error_over_type_prev: ", $e2p::class, "\n";
echo "type_inner_prev_null: ", ($e2p->getPrevious() === null ? 1 : 0), "\n";

$chain = new RuntimeException("r1", 0, new LogicException("l2", 0, new DivisionByZeroError("d3")));
$second = $chain->getPrevious();
$third = $second->getPrevious();
echo "chain_second: ", $second::class, "\n";
echo "chain_third: ", $third::class, "\n";
echo "chain_third_msg: ", $third->getMessage(), "\n";
echo "chain_fourth_null: ", ($third->getPrevious() === null ? 1 : 0), "\n";
echo "chain_three_calls_null: ", ($chain->getPrevious()->getPrevious()->getPrevious() === null ? 1 : 0), "\n";

echo "error_is_throwable: ", ($err instanceof Throwable ? 1 : 0), "\n";
echo "error_has_no_previous: ", ($err->getPrevious() === null ? 1 : 0), "\n";

$wrapped = new ErrorException("wrapped", 1, E_WARNING, null, null, $err);
$wp = $wrapped->getPrevious();
echo "errorexception_prev_class: ", $wp::class, "\n";
echo "errorexception_prev_is_err: ", ($wp === $err ? 1 : 0), "\n";
echo "end\n";

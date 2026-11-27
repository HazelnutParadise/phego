<?php
// fatal_throw_uncaught_expression: throw used as an expression (null coalescing, or, arrow function), then one uncaught throw
$set = "value";
$x = null;
$first = $set ?? throw new LogicException("never thrown");
echo "first: ", $first, "\n";
$flag = true;
$ok = $flag || throw new LogicException("not thrown either");
echo "or_short_circuit: ", $ok ? "yes" : "no", "\n";
$fn = fn($v) => $v ?: throw new LogicException("empty");
$kept = $fn("kept");
echo "arrow_truthy: ", $kept, "\n";
echo "start\n";
$y = $x ?? throw new LogicException("none");
echo "never\n";

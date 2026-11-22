<?php
// throw_variable_and_ternary: throw of a variable, a ternary, a function result and a dynamic class name
class A extends Exception {}
class B extends Exception {}

function make_one() {
    return new RuntimeException("from function");
}

$e1 = new Exception("var");
try {
    throw $e1;
} catch (Exception $c) {
    echo "variable_same: ", ($c === $e1 ? 1 : 0), "\n";
    echo "variable_message: ", $c->getMessage(), "\n";
}

foreach ([true, false] as $cond) {
    try {
        throw $cond ? new A("a") : new B("b");
    } catch (A $x) {
        echo "ternary_true_class: ", $x::class, " ", $x->getMessage(), "\n";
    } catch (B $x) {
        echo "ternary_false_class: ", $x::class, " ", $x->getMessage(), "\n";
    }
}

try {
    throw make_one();
} catch (RuntimeException $r) {
    echo "function_result: ", $r->getMessage(), "\n";
}

$c = 'RuntimeException';
try {
    throw new $c("x");
} catch (RuntimeException $rr) {
    echo "dynamic_class: ", $rr::class, " ", $rr->getMessage(), "\n";
}

$cls = 'LogicException';
try {
    throw new $cls("y", 3);
} catch (Exception $z) {
    echo "dynamic_logic_is_logic: ", ($z instanceof LogicException ? 1 : 0), "\n";
    echo "dynamic_code: ", $z->getCode(), "\n";
}

$holder = [new B("in array")];
try {
    throw $holder[0];
} catch (B $bb) {
    echo "array_element_thrown: ", $bb->getMessage(), "\n";
}
echo "end\n";

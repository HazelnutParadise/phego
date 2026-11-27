<?php
// try_catch_case_insensitive: catch class names match case-insensitively, for user and built-in classes
class MyFault extends Exception {}

try {
    throw new MyFault("one");
} catch (myfault $e) {
    echo "lower catch: ", $e->getMessage(), "\n";
}

try {
    throw new MyFault("two");
} catch (MYFAULT $e) {
    echo "upper catch: ", $e->getMessage(), "\n";
}

try {
    throw new RuntimeException("three");
} catch (runtimeexception $e) {
    echo "runtime lower: ", $e->getMessage(), "\n";
}

try {
    throw new \exception("four");
} catch (\EXCEPTION $e) {
    echo "global upper: ", $e->getMessage(), "\n";
}

try {
    throw new MyFault("five");
} catch (\Exception $e) {
    echo "parent mixed case: ", $e->getMessage(), "\n";
}

$m = new MYFAULT("x");
echo "declared name: ", $m::class, "\n";
echo "instanceof lower: ", $m instanceof myfault ? "yes" : "no", "\n";

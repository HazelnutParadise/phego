<?php
// try_catch_inheritance: catch by parent class, grandparent, interface, Throwable and Stringable; Error is not caught by catch (Exception)
interface Coded {}
class Base extends Exception implements Coded {}
class Mid extends Base {}
class Leaf extends Mid {}

try {
    throw new Leaf("a");
} catch (Base $e) {
    echo "Leaf caught by Base: ", $e->getMessage(), "\n";
}
try {
    throw new Leaf("b");
} catch (Exception $e) {
    echo "Leaf caught by Exception: ", $e::class, "\n";
}
try {
    throw new Leaf("c");
} catch (Coded $e) {
    echo "Leaf caught by interface Coded: ", $e::class, "\n";
}
try {
    throw new Mid("d");
} catch (Stringable $e) {
    echo "Mid caught by Stringable: ", $e->getMessage(), "\n";
}
try {
    throw new Error("e");
} catch (Throwable $e) {
    echo "Error caught by Throwable: ", $e::class, "\n";
}
try {
    try {
        throw new Error("f");
    } catch (Exception $e) {
        echo "wrong: Exception caught Error\n";
    }
} catch (Error $e) {
    echo "outer caught Error: ", $e->getMessage(), "\n";
}
$probe = new Leaf("x");
echo "Leaf is Coded: ", $probe instanceof Coded ? "yes" : "no", "\n";
echo "Leaf is Error: ", $probe instanceof Error ? "yes" : "no", "\n";

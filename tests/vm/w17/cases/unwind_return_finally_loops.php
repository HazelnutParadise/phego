<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
function a() {
    foreach ([1, new D(1)] as $x) {
        try { return 1; } finally { echo "fin\n"; }
    }
}
echo a(), "\n";
function b() {
    foreach ([1, new D(2)] as $x) {
        try {
            foreach ([1, new D(3)] as $y) {
                try { return 2; } finally { echo "fin-inner\n"; }
            }
        } finally { echo "fin-outer\n"; }
    }
}
echo b(), "\n";
class T { function __destruct() { echo "~T\n"; throw new Exception("T"); } }
function c() {
    try {
        foreach ([1, new T] as $x) {
            try { return 3; } finally { echo "fin-c\n"; }
        }
    } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; return 4; }
}
echo c(), "\n";

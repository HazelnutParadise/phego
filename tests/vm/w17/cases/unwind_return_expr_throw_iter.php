<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
class S { function __toString(): string { throw new Error("str"); } }
function f($o) {
    try {
        foreach ([1, new D(1)] as $a) {
            foreach ([1, new D(2)] as $b) {
                try { return "s" . $o; } catch (Exception $e) {}
            }
        }
    } finally { echo "fin\n"; }
}
try { f(new S); } catch (Error $e) { echo "caught\n"; }
function g($o) {
    try {
        foreach ([1, new D(3)] as $a) {
            foreach ([1, new D(4)] as $b) {
                return "s" . $o;
            }
        }
    } finally { echo "fin\n"; }
}
try { g(new S); } catch (Error $e) { echo "caught\n"; }

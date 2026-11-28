<?php
class D { function __construct(public $n) {} function __destruct() { echo "~D{$this->n}\n"; } }
class T { function __destruct() { echo "~T\n"; throw new Exception("T"); } }
function a() {
    try {
        foreach ([1, new D(1)] as $x) {
            try { return 1; } finally { echo "F1\n"; }
        }
    } finally { echo "F2\n"; }
}
echo a(), "\n";
function b() {
    try {
        try {
            foreach ([1, new T] as $x) {
                try { return 2; } finally { echo "F1\n"; }
            }
        } catch (Exception $e) { echo "caught ", $e->getMessage(), "\n"; return 3; }
    } finally { echo "F2\n"; }
}
echo b(), "\n";
function c() {
    try {
        foreach ([1, new D(2)] as $x) {
            foreach ([1, new D(3)] as $y) {
                try { return new D(9); } finally { echo "F1\n"; }
            }
        }
    } finally { echo "F2\n"; }
}
$r = c(); echo "got\n"; unset($r);

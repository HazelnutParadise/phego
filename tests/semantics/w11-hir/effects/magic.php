<?php
// Each line after the marker is one statement inside try; effects.php
// records the user callbacks, warnings and exceptions it causes. Every
// function above calls entered() so the harness sees it run.
class Magic {
    public function __get($n) { entered(); return 1; }
    public function __set($n, $v) { entered();}
    public function __isset($n) { entered(); return true; }
    public function __unset($n) { entered();}
    public function __call($n, $a) { entered();}
    public static function __callStatic($n, $a) { entered();}
    public function __toString(): string { entered(); return 's'; }
    public function __invoke() { entered();}
    public function __clone() { entered();}
}
class Access implements ArrayAccess {
    public function offsetGet($o): mixed { entered(); return 1; }
    public function offsetSet($o, $v): void { entered();}
    public function offsetExists($o): bool { entered(); return true; }
    public function offsetUnset($o): void { entered();}
}
class Dtor { public function __destruct() { entered();} }
class It implements Iterator {
    private $i = 0;
    public function current(): mixed { entered(); return $this->i; }
    public function key(): mixed { entered(); return $this->i; }
    public function next(): void { entered(); $this->i++; }
    public function rewind(): void { entered(); $this->i = 0; }
    public function valid(): bool { entered(); return $this->i < 2; }
}
spl_autoload_register(function ($c) { entered(); if (str_starts_with($c, 'Lazy')) { eval("namespace Lazy; class " . substr($c, 5) . " { const C = 1; public static \$p = 1; static function f() { entered();} }"); } });
$o = new Magic; $arr = new Access; $it = new It;
// --- snippets ---
try { $o->undefinedProp; } catch (\Throwable $e) { thrown(__LINE__); }
try { $o->x = 1; } catch (\Throwable $e) { thrown(__LINE__); }
try { isset($o->y); } catch (\Throwable $e) { thrown(__LINE__); }
try { empty($o->y); } catch (\Throwable $e) { thrown(__LINE__); }
try { $o->y ?? 0; } catch (\Throwable $e) { thrown(__LINE__); }
try { unset($o->z); } catch (\Throwable $e) { thrown(__LINE__); }
try { $o->method(); } catch (\Throwable $e) { thrown(__LINE__); }
try { Magic::stat(); } catch (\Throwable $e) { thrown(__LINE__); }
try { echo $o; } catch (\Throwable $e) { thrown(__LINE__); }
try { $s = "v=$o"; } catch (\Throwable $e) { thrown(__LINE__); }
try { $s = $o . ''; } catch (\Throwable $e) { thrown(__LINE__); }
try { $s = (string) $o; } catch (\Throwable $e) { thrown(__LINE__); }
try { $b = $o == 's'; } catch (\Throwable $e) { thrown(__LINE__); }
try { $arr['k']; } catch (\Throwable $e) { thrown(__LINE__); }
try { $arr['k'] = 1; } catch (\Throwable $e) { thrown(__LINE__); }
try { $arr[] = 1; } catch (\Throwable $e) { thrown(__LINE__); }
try { isset($arr['k']); } catch (\Throwable $e) { thrown(__LINE__); }
try { unset($arr['k']); } catch (\Throwable $e) { thrown(__LINE__); }
try { $arr['k'] ??= 2; } catch (\Throwable $e) { thrown(__LINE__); }
try { $d = new Dtor; $d = null; } catch (\Throwable $e) { thrown(__LINE__); }
try { $d = new Dtor; unset($d); } catch (\Throwable $e) { thrown(__LINE__); }
try { $d = new Dtor; $d = 1; } catch (\Throwable $e) { thrown(__LINE__); }
try { $d = new Dtor; $e2 = &$d; $e2 = 2; } catch (\Throwable $e) { thrown(__LINE__); }
try { new Lazy\Thing; } catch (\Throwable $e) { thrown(__LINE__); }
try { Lazy\Other::C; } catch (\Throwable $e) { thrown(__LINE__); }
try { Lazy\Third::$p; } catch (\Throwable $e) { thrown(__LINE__); }
try { Lazy\Fourth::f(); } catch (\Throwable $e) { thrown(__LINE__); }
try { $x = $q instanceof Lazy\Never; } catch (\Throwable $e) { thrown(__LINE__); }
try { foreach ($it as $v) {} } catch (\Throwable $e) { thrown(__LINE__); }
try { $x = [...$it]; } catch (\Throwable $e) { thrown(__LINE__); }
try { $c = clone $o; } catch (\Throwable $e) { thrown(__LINE__); }
try { $o(); } catch (\Throwable $e) { thrown(__LINE__); }
try { $f = function () use ($undefinedA) {}; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = $undefinedB; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = $undefinedC[1]; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = [1][5]; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = "5 apples" + 1; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = "abc" * 1; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = 1 % 0; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = 1 / 0; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = 1 << -1; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = [] + 1; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = -[]; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = (int) 1e30; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = (int) $o; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = [1.5 => 'a']; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = match (5) { 1 => 0 }; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = UNDEFINED_CONST; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = strlen(); } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = array_map(fn($v) => $v, [1]); } catch (\Throwable $e) { thrown(__LINE__); }
try { $n = null; $n->m(); } catch (\Throwable $e) { thrown(__LINE__); }
try { $n = null; $n?->m(); } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = 1 + 2; $u = 'a' . 'b'; $u = !$u; } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = ['a' => 1, 2]; $u = isset($missing); } catch (\Throwable $e) { thrown(__LINE__); }
try { $u = PHP_INT_MAX + 1; $u = 7 <=> 8; } catch (\Throwable $e) { thrown(__LINE__); }

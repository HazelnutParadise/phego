<?php
// catch_engine_error_then_continue: twenty different engine errors raised one after another, each caught, and the state stays usable
class Obj
{
    private function secret()
    {
        return 1;
    }
}

abstract class Abs {}
class RO
{
    public function __construct(public readonly int $v)
    {
    }
}

function fail_at($i)
{
    $zero = 0;
    $arr = [1];
    $s = "abc";
    $o = new Obj();
    $n = null;
    $ro = new RO(1);
    return match ($i) {
        0 => 1 / $zero,
        1 => 1 % $zero,
        2 => no_such_fn(),
        3 => new Missing(),
        4 => $o->nope(),
        5 => $o->secret(),
        6 => $arr + 1,
        7 => $s * 2,
        8 => $n->m(),
        9 => (5)->m(),
        10 => new Abs(),
        11 => UNDEFINED_CONST_X,
        12 => Obj::$nope,
        13 => 1 << -$i,
        14 => $ro->v = 2,
        15 => $arr->m(),
        16 => $zero->m(),
        17 => match ($i) { 99 => 1 },
        18 => (new Closure()),
        19 => $s[0][0] = [],
    };
}

for ($i = 0; $i < 20; $i++) {
    try {
        $r = fail_at($i);
        echo $i, ": no error\n";
    } catch (Error $e) {
        echo $i, ": ", $e::class, "\n";
    }
}
$after = 7;
echo "usable_after_all: ", $after * 6, "\n";
echo "end: ok\n";

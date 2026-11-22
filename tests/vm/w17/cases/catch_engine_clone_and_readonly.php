<?php
// catch_engine_clone_and_readonly: clone of a non-object is an Error; readonly changes are allowed in __clone but not outside
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class Pt
{
    public function __construct(public readonly int $x)
    {
    }

    public function __clone()
    {
        $this->x = $this->x + 1;
    }
}

$num = 5;
try {
    $c = clone $num;
} catch (Error $e) {
    report("clone_scalar", $e);
}

$p = new Pt(10);
$q = clone $p;
echo "clone_in_clone_method: ", $q->x, "\n";
echo "original_unchanged: ", $p->x, "\n";

try {
    $p->x = 20;
} catch (Error $e) {
    report("readonly_outside", $e);
}

try {
    $p->x++;
} catch (Error $e) {
    report("readonly_increment", $e);
}
echo "still: ", $p->x, "\n";
echo "end: ok\n";

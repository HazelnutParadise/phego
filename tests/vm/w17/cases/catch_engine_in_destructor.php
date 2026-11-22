<?php
// catch_engine_in_destructor: an engine error raised in a destructor (on unset) and in __toString (on echo) is caught around the triggering statement
function report($label, $e)
{
    echo $label, ": ", $e::class, " | ", $e->getMessage(), " | line ", $e->getLine(), "\n";
}

class Bad
{
    public function __destruct()
    {
        echo "destructor_start\n";
        $zero = 0;
        $r = 1 % $zero;
        echo "destructor_not_reached\n";
    }
}

class Str
{
    public function __toString(): string
    {
        $arr = [];
        $x = $arr - 1;
        return "unreached";
    }
}

$b = new Bad();
try {
    unset($b);
    echo "after_unset_statement\n";
} catch (DivisionByZeroError $e) {
    report("destructor", $e);
}
echo "b_is_gone: ", isset($b) ? "no" : "yes", "\n";

$s = new Str();
try {
    $text = "value: " . $s;
    echo $text, "\n";
} catch (TypeError $e) {
    report("tostring", $e);
}
echo "end: ok\n";

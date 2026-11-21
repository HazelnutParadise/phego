<?php
class Typed
{
    public int $lazy;
    public function __get($n)
    {
        echo "__get(", $n, ")\n";
        return "not an int";
    }
}
$t = new Typed;
unset($t->lazy);
echo "reading\n";
echo $t->lazy, "\n";

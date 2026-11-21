<?php
function implicit(int $x = null)
{
    return $x ?? "null";
}
function explicitly(?int $x = null)
{
    return $x ?? "null";
}
class C
{
    public function m(string $s = null, ?array $a = null)
    {
        return ($s ?? "s-null") . " " . ($a === null ? "a-null" : "a");
    }
}
echo implicit(), " ", implicit(4), " ", explicitly(), "\n";
echo (new C)->m(), " ", (new C)->m("x", []), "\n";

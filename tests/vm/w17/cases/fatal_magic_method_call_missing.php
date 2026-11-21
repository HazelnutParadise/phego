<?php
class OnlyStatic
{
    public static function __callStatic($n, $a)
    {
        return "static " . $n;
    }
}
echo OnlyStatic::thing(), "\n";
$o = new OnlyStatic;
echo $o->thing(), "\n";

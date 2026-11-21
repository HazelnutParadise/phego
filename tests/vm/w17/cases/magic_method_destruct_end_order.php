<?php
class Res
{
    public function __construct(public $name)
    {
    }

    public function __destruct()
    {
        echo "destruct ", $this->name, "\n";
    }
}

$first = new Res("first global");
$second = new Res("second global");
$arr = [new Res("array 0"), new Res("array 1")];
$third = new Res("third global");
$nested = new Res("outer");
$nested->inner = new Res("inner");
$shared = new Res("shared");
$alias = $shared;

class Keeper
{
    public static $kept;
}
Keeper::$kept = new Res("static property");

function with_static()
{
    static $s;
    $s = new Res("static variable");
}
with_static();
echo "end of script\n";

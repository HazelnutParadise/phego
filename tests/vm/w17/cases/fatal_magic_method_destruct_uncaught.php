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

function fail()
{
    $local = new Res("function local");
    echo "failing\n";
    return undefined_fn();
}

$g = new Res("global");
unset($g);
$kept = new Res("kept global");
fail();

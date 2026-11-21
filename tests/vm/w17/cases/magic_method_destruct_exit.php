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

function work()
{
    $local = new Res("function local");
    echo "exiting inside function\n";
    exit(4);
}

$g = new Res("global");
$h = new Res("second global");
work();
echo "not reached\n";

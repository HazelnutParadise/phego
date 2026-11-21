<?php
class Res
{
    public function __construct(public $name)
    {
        echo "open ", $name, "\n";
    }

    public function __destruct()
    {
        echo "close ", $this->name, "\n";
    }

    public function use()
    {
        echo "use ", $this->name, "\n";
        return $this;
    }

    public function value()
    {
        return "value of " . $this->name;
    }
}

function scoped()
{
    $r = new Res("local");
    echo "in function\n";
}
scoped();
echo "after function\n";

function make($n)
{
    return new Res($n);
}
make("discarded return");
echo "after discarded return\n";

new Res("temporary");
echo "after temporary statement\n";

(new Res("chained"))->use();
echo "after chained temporary\n";

echo (new Res("in echo"))->value(), "\n";
echo "after echo temporary\n";

$kept = make("kept");
echo "kept alive\n";

function returns_param($r)
{
    return $r;
}
returns_param(new Res("through param"));
echo "after param round trip\n";

function inner_loop()
{
    foreach ([1, 2] as $i) {
        $r = new Res("loop " . $i);
    }
    echo "loop done\n";
}
inner_loop();
echo "end\n";

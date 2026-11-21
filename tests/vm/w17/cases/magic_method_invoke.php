<?php
class Multiplier
{
    public function __construct(private $factor)
    {
    }

    public function __invoke($x, $y = 1)
    {
        return $x * $y * $this->factor;
    }
}

class Logger
{
    public $lines = [];
    public function __invoke(...$parts)
    {
        $line = "";
        foreach ($parts as $p) {
            $line .= $p;
        }
        $this->lines[] = $line;
        return $this;
    }
}

$double = new Multiplier(2);
echo $double(5), "\n";
echo $double(3, 4), "\n";
echo $double(y: 10, x: 1), "\n";
echo $double(...[2, 2]), "\n";
$l = new Logger;
$l("a", "b")("c")("d", "e", "f");
foreach ($l->lines as $line) {
    echo "line ", $line, "\n";
}
$holder = ["m" => new Multiplier(3)];
echo $holder["m"](7), "\n";
echo (new Multiplier(10))(1), "\n";
echo "explicit: ", $double->__invoke(6), "\n";

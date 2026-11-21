<?php
class Point
{
    public $x = 0;
    public $y = 0;
    public $label = "origin";

    public function move($dx, $dy)
    {
        $this->x += $dx;
        $this->y += $dy;
        return $this;
    }

    public function describe()
    {
        return $this->label . "(" . $this->x . ", " . $this->y . ")";
    }
}

$p = new Point();
echo "default: ", $p->describe(), "\n";
$p->move(2, 3);
echo "moved: ", $p->describe(), "\n";
$p->label = "p";
$p->move(-1, 1)->move(10, 0);
echo "chained: ", $p->describe(), "\n";

$q = new Point;
echo "independent: ", $q->describe(), "\n";
$q->x = 5;
echo "p.x=", $p->x, " q.x=", $q->x, "\n";

class Counter
{
    public $n = 0;
    function inc()
    {
        return ++$this->n;
    }
}
$c = new Counter;
echo "implicit public: ", $c->inc(), $c->inc(), $c->inc(), "\n";

class EmptyOne
{
}
$e = new EmptyOne;
echo "empty class instanceof: ", ($e instanceof EmptyOne ? "yes" : "no"), "\n";

function make_point($x, $y)
{
    $p = new Point;
    $p->x = $x;
    $p->y = $y;
    $p->label = "made";
    return $p;
}
echo make_point(7, 8)->describe(), "\n";
echo (new Point)->move(1, 1)->describe(), "\n";

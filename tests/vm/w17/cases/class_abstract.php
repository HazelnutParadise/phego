<?php
abstract class Shape
{
    protected $name;

    public function __construct($name)
    {
        $this->name = $name;
    }

    abstract public function area();
    abstract protected function unit();

    public function report()
    {
        return $this->name . " area " . $this->area() . " " . $this->unit();
    }

    public static function kind()
    {
        return "shape";
    }
}

class Rect extends Shape
{
    private $w;
    private $h;

    public function __construct($w, $h)
    {
        parent::__construct("rect");
        $this->w = $w;
        $this->h = $h;
    }

    public function area()
    {
        return $this->w * $this->h;
    }

    public function unit()
    {
        return "cm2";
    }
}

abstract class Middle extends Shape
{
    protected function unit()
    {
        return "m2";
    }
}

final class Square extends Middle
{
    public function __construct(private $side)
    {
        parent::__construct("square");
    }

    public function area()
    {
        return $this->side * $this->side;
    }
}

$r = new Rect(3, 4);
echo $r->report(), "\n";
echo $r->unit(), "\n";
$s = new Square(5);
echo $s->report(), "\n";
echo "static on abstract: ", Shape::kind(), "\n";
echo "static via child: ", Square::kind(), "\n";
echo "instanceof Shape: ", ($s instanceof Shape ? "yes" : "no"), "\n";
echo "instanceof Middle: ", ($s instanceof Middle ? "yes" : "no"), "\n";
echo "rect instanceof Middle: ", ($r instanceof Middle ? "yes" : "no"), "\n";

abstract class NoAbstractMethods
{
    public $v = "concrete member of abstract class";
}
class Concrete extends NoAbstractMethods
{
}
echo (new Concrete)->v, "\n";

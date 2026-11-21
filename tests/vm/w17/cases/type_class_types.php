<?php
interface Shape
{
    public function area(): float;
}
class Square implements Shape
{
    public function __construct(private float $s)
    {
    }
    public function area(): float
    {
        return $this->s * $this->s;
    }
    public function grow(): static
    {
        return new static($this->s + 1);
    }
    public function same(self $o): bool
    {
        return $o->s === $this->s;
    }
}
class Big extends Square
{
}

function total(Shape ...$shapes): float
{
    $t = 0.0;
    foreach ($shapes as $s) {
        $t += $s->area();
    }
    return $t;
}
function maybe(?Square $s = null): string
{
    return $s === null ? "none" : "square " . $s->area();
}
function obj(object $o): string
{
    return "object " . $o::class;
}
function iter(iterable $it): string
{
    $out = "";
    foreach ($it as $k => $v) {
        $out .= $k . "=" . $v . ";";
    }
    return $out;
}
function either(Square|int $x): string
{
    return $x instanceof Square ? "square" : "int " . $x;
}
function ret_iface(): Shape
{
    return new Big(2);
}

echo total(new Square(2), new Big(3)), "\n";
echo maybe(), " | ", maybe(new Big(1)), "\n";
echo obj(new Square(1)), "\n";
echo iter([1, "a" => 2]), "\n";
echo either(new Big(1)), " ", either("7"), "\n";
echo ret_iface()->area(), " ", ret_iface()::class, "\n";
echo (new Big(1))->grow()::class, " ", (new Big(1))->grow()->area(), "\n";
echo ((new Square(2))->same(new Big(2)) ? "same" : "diff"), "\n";
echo "case-insensitive type name: ", lower_type(new Square(1)), "\n";
function lower_type(square $s)
{
    return "ok";
}

<?php
interface Reader
{
    public function read(int $n): string;
}

class Base implements Reader
{
    public function read(int $n, $extra = "x"): string
    {
        return "base read " . $n . $extra;
    }

    protected function widen()
    {
        return "protected in base";
    }

    public function ret(): ?Base
    {
        return null;
    }

    public function param(Child $c)
    {
        return "base param";
    }
}

class Child extends Base
{
    public function read(int|string $n, $extra = "y", ...$more): string
    {
        return "child read " . $n . $extra;
    }

    public function widen()
    {
        return "public in child";
    }

    public function ret(): Child
    {
        return $this;
    }

    public function param(Base $c)
    {
        return "child param accepts wider type";
    }
}

$c = new Child;
echo $c->read(1), "\n";
echo $c->read("two", "!"), "\n";
echo $c->widen(), "\n";
echo ($c->ret() === $c ? "covariant return ok" : "no"), "\n";
echo $c->param(new Base), "\n";
echo (new Base)->read(3), "\n";
echo ((new Base)->ret() === null ? "base returns null" : "no"), "\n";

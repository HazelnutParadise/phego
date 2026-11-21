<?php
class Temp
{
    private $deg = 0;

    public function set($d): static
    {
        $this->deg = $d;
        return $this;
    }

    public function copy(): self
    {
        $t = new self;
        $t->deg = $this->deg;
        return $t;
    }

    public function get(): int
    {
        return $this->deg;
    }

    public function compare(Temp $o): int
    {
        return $this->deg <=> $o->deg;
    }

    public function thisIs($o)
    {
        return $this === $o ? "same" : "different";
    }
}

$t = (new Temp)->set(20);
$u = $t->copy()->set(30);
echo $t->get(), " ", $u->get(), " ", $t->compare($u), " ", $u->compare($t), "\n";
echo $t->thisIs($t), " ", $t->thisIs($u), "\n";

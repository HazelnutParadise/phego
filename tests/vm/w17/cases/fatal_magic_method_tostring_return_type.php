<?php
class Num
{
    public function __construct(private $v)
    {
    }
    public function __toString(): string
    {
        return $this->v;
    }
}
echo new Num("ok"), "\n";
echo new Num(5), "\n";
echo new Num([]), "\n";

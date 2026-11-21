<?php
class P
{
    public function __construct(public $x = 0)
    {
    }
}
echo (new P(x: 1))->x, "\n";
new P(y: 2);

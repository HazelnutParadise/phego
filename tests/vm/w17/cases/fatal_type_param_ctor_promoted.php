<?php
class P
{
    public function __construct(public int $x)
    {
    }
}
echo (new P("5"))->x, "\n";
new P("five");

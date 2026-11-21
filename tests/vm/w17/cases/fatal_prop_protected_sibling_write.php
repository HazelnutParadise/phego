<?php
class Left
{
    protected $own = "left";
}
class Right
{
    public function poke(Left $l)
    {
        $l->own = "changed";
    }
}
echo "poking\n";
(new Right)->poke(new Left);

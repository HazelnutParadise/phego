<?php
class Doc
{
    public function __clone()
    {
        echo "cloning\n";
        $this->x->y = 1;
    }
}
$d = new Doc;
echo "start\n";
$e = clone $d;

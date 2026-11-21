<?php
class Holder
{
    public $inner = null;
}
$h = new Holder;
$h->a = 1;
echo "dynamic works\n";
$h->inner->x = 1;

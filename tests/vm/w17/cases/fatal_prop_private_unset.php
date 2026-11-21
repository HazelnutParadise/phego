<?php
class Box
{
    private $content = "x";
}
$b = new Box;
echo "unsetting\n";
unset($b->content);

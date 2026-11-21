<?php
interface Shape
{
    const KIND = "shape";
}
echo Shape::KIND, "\n";
$s = new Shape;

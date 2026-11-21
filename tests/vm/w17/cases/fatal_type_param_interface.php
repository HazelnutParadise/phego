<?php
interface Printable
{
}
class Doc implements Printable
{
}
class Img
{
}
function output(Printable $p)
{
    return "printed";
}
echo output(new Doc), "\n";
echo output(new Img), "\n";

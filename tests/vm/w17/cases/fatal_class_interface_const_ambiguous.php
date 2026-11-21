<?php
echo "start\n";
interface First
{
    const X = 1;
}
interface Second
{
    const X = 2;
}
class Both implements First, Second
{
}

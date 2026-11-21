<?php
echo "start\n";
interface Config
{
    final const MODE = "a";
}
class Overrider implements Config
{
    const MODE = "b";
}

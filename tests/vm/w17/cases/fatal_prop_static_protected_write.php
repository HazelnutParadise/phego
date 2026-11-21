<?php
class Registry
{
    protected static $count = 0;
}
echo "writing\n";
Registry::$count = 5;

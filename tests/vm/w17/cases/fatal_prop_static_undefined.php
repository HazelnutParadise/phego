<?php
class Registry
{
    public static $items = [];
}
Registry::$items[] = 1;
echo "items ok\n";
echo Registry::$missing, "\n";

<?php
class Registry
{
    public static int $count = 0;
}
Registry::$count = "5";
echo "count ", Registry::$count, "\n";
Registry::$count = "five";

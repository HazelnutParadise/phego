<?php
class Config
{
    public int $port = 80;
}
$c = new Config;
unset($c->port);
echo "unset done\n";
$c->port++;

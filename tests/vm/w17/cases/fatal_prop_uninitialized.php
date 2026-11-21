<?php
class Config
{
    public string $host;
    public int $port = 80;
}
$c = new Config;
echo "port ", $c->port, "\n";
echo "isset host: ", (isset($c->host) ? "yes" : "no"), "\n";
echo $c->host, "\n";

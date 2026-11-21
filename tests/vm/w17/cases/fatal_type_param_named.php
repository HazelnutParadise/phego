<?php
function cfg(string $host = "h", int $port = 80)
{
    return $host . ":" . $port;
}
echo cfg(port: 8080), "\n";
echo cfg(port: "http"), "\n";

<?php
class Router
{
    public function get()
    {
        return "GET";
    }
}
$r = new Router;
$verb = "get";
echo $r->$verb(), "\n";
$verb = "delete";
echo $r->$verb(), "\n";

<?php
class Api
{
    public function call($endpoint, $method = "GET")
    {
        return $method . " " . $endpoint;
    }
}
$a = new Api;
echo $a->call(method: "POST", endpoint: "/x"), "\n";
echo $a->call("/y", verb: "PUT"), "\n";

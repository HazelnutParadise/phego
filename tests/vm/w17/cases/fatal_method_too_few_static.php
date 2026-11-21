<?php
class Mailer
{
    public static function send($to)
    {
        return "sent";
    }
}
function go()
{
    return Mailer::send();
}
echo "going\n";
go();

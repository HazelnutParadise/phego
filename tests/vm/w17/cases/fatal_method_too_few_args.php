<?php
class Mailer
{
    public function send($to, $subject, $body = "")
    {
        return "sent " . $to . " " . $subject;
    }
}
$m = new Mailer;
echo $m->send("a", "b"), "\n";
echo $m->send("only to"), "\n";

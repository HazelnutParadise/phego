<?php
declare(strict_types=1);
class Label
{
    public string $text = "";
}
$l = new Label;
$l->text = "ok";
echo "string ok\n";
$l->text = 5;

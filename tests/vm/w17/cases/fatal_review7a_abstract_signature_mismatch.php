<?php
trait T { abstract function need(int $x): string; }
class C { use T; function need(string $x): string { return ""; } }

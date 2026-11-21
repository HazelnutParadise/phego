<?php
trait T { abstract function m(); }
class C { use T; static function m() {} }

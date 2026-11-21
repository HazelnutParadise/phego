<?php
trait T { abstract static function m(); }
class C { use T; function m() {} }

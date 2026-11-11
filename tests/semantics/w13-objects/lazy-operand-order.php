<?php
// REQ-018; T-035. The compare-lazy-operand-owners case of review.php on its
// own, for PHP builds outside the pinned Linux images (macOS, Windows): the
// order in which == initializes two lazy operands follows the C compiler's
// argument evaluation order. Prints the events as one JSON line.
class W13CompareHeld {
    public int $i;
    public string $label;
    public function __destruct(){$GLOBALS['review_events'][]='dtor:'.$this->label;}
}
$GLOBALS['review_events']=[];$r=new ReflectionClass(W13CompareHeld::class);
$GLOBALS['review_left']=$r->newLazyGhost(function($o){$GLOBALS['review_events'][]='init:A';unset($GLOBALS['review_left'],$GLOBALS['review_right']);$o->i=1;$o->label='A';});
$GLOBALS['review_right']=$r->newLazyGhost(function($o){$GLOBALS['review_events'][]='init:B';$o->i=1;$o->label='B';});
$same=$GLOBALS['review_left']==$GLOBALS['review_right'];
echo json_encode([$same,$GLOBALS['review_events']]),"\n";

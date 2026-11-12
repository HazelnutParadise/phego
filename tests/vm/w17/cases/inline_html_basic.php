<html>
<?php echo "one"; ?>
<p>after one</p>
<?php echo "two" ?>

<p>blank line kept after two</p>
<?php if (1 > 0) { ?>
  <b>inside if</b>
<?php } else { ?>
  <b>never</b>
<?php } ?>
<?php echo "plain echo\n" ?>
end<?php echo "!"; ?>

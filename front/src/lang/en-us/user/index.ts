export default {
  'user.title': 'User',
  'user.desc': 'User management',
  'user.form.title': 'Sign up',
  'user.form.user': 'Username',
  'user.form.password': 'Password',
  'user.form.confirm': 'Confirm',
  'user.form.real_name': 'RealName',
  'user.form.role': 'Role',
  'user.form.dept': 'Dept',
  'user.form.email': 'Email',
  'user.form.recorder': 'Auditor',
  'user.form.valid.valid': 'The entered passwords are inconsistent',
  'user.form.valid.user': 'Please enter username',
  'user.form.valid.password':
    'Contain at least one uppercase letter, one lowercase letter, and one digit',
  'user.form.valid.confirm': 'Please enter confirm',
  'user.form.valid.real_name': 'Please enter realname',
  'user.form.valid.dept': 'Please enter dept',
  'user.form.valid.role': 'Please select role',
  'user.form.valid.email': 'Please enter a correct mail format',
  'user.form.delete.tips': 'Are you sure delete the user?',

  'user.role.auditor': 'Auditor',
  'user.role.guest': 'Guest',

  'user.policy': 'User policy',

  'user.username': 'Username',
  'user.password': 'Password',
  'user.password.title': 'Change password',
  'user.password.old': 'Old password',
  'user.password.new': 'New password',
  'user.password.confirm': 'Confirm new password',
  'user.mfa.title': 'MFA (TOTP)',
  'user.mfa.on': 'Enabled',
  'user.mfa.off': 'Disabled',
  'user.mfa.intro':
    'Add TOTP two-factor authentication (Google Authenticator / 1Password etc.). A stolen password alone will no longer be enough to sign in.',
  'user.mfa.start': 'Enroll',
  'user.mfa.setup.tips':
    'Scan the QR code with an authenticator app (or enter the secret manually), then enter the 6-digit code shown in the app to confirm.',
  'user.mfa.manual': 'Manual entry secret',
  'user.mfa.code': '6-digit code',
  'user.mfa.enable': 'Confirm & enable',
  'user.mfa.regen': 'Regenerate secret',
  'user.mfa.bound.tips':
    'A code from your authenticator is required after the password at sign-in. To switch devices, unbind first and re-enroll.',
  'user.mfa.disable': 'Unbind',
  'user.mfa.disable.tips': 'Sign-in will no longer require a code. Unbind?',
  'user.mfa.login.tips':
    'Enter the 6-digit code from your authenticator to finish signing in',
  'user.mfa.back': 'Back to sign-in',
  'user.mfa.reset': 'Reset MFA',
  'user.mfa.reset.tips':
    "Reset this user's MFA binding? After reset the next sign-in needs the password only (the unlock path for a lost authenticator).",
};

use std::io::{self, Stdout};

use anyhow::Result;
use crossterm::{
    event::{self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use tui::{
    backend::{Backend, CrosstermBackend},
    layout::{Constraint, Layout},
    style::{Color, Modifier, Style},
    widgets::{Block, Borders, Cell, Row, Table, TableState},
    Frame, Terminal as TuiTerminal,
};
use venus_worker_util::sealcalc;

/// Display the status of tasks running at different times by tui's table
pub fn display(
    items: &[sealcalc::Item],
    pc1_concurrent: usize,
    pc2_concurrent: usize,
    c2_concurrent: usize,
    sealing_threads: usize,
) -> Result<()> {
    App {
        items,
        pc1_concurrent,
        pc2_concurrent,
        c2_concurrent,
        sealing_threads,
        state: Default::default(),
    }
    .run()
}

struct App<'a> {
    items: &'a [sealcalc::Item],
    pc1_concurrent: usize,
    pc2_concurrent: usize,
    c2_concurrent: usize,
    sealing_threads: usize,

    state: TableState,
}

impl<'a> App<'a> {
    pub fn run(&mut self) -> Result<()> {
        let mut terminal = setup_terminal();

        loop {
            terminal.inner_mut().draw(|f| self.ui(f))?;

            if let Event::Key(key) = event::read()? {
                match key.code {
                    KeyCode::Char('q') => return Ok(()),
                    KeyCode::Down => self.next(1),
                    KeyCode::Up => self.next(-1),
                    KeyCode::Left => self.next(-20),
                    KeyCode::Char(' ') | KeyCode::Right => self.next(20),
                    _ => {}
                }
            }
        }
    }

    fn next(&mut self, size: isize) {
        let i = match self.state.selected() {
            Some(i) => {
                let i = i as isize;
                let len = self.items.len() as isize;
                if i == len - 1 {
                    0
                } else if i >= len - size {
                    (self.items.len() - 1) as isize
                } else {
                    (i + size + len) % len
                }
            }
            None => size,
        };
        self.state.select(Some(i as usize));
    }

    fn ui<B: Backend>(&mut self, f: &mut Frame<B>) {
        let rects = Layout::default()
            .constraints([Constraint::Percentage(100)].as_ref())
            .split(f.size());

        let selected_style = Style::default().add_modifier(Modifier::REVERSED);
        let normal_style = Style::default().bg(Color::Blue);
        let header_cells = [
            "time (mins)",
            "sealing threads\n(free/total)",
            "    pc1\n(free/total)",
            "    pc2\n(free/total)",
            "wait seed",
            "    c2\n(free/total)",
            "finished\nsectors",
        ]
        .iter()
        .map(|h| Cell::from(*h).style(Style::default().fg(Color::LightYellow)));

        let header = Row::new(header_cells)
            .style(normal_style)
            .height(2)
            .bottom_margin(1);

        let rows = self.items.iter().map(|item| {
            Row::new([
                Cell::from(item.time_in_mins.to_string()),
                Cell::from(format!(
                    "{}/{}",
                    item.sealing_threads_running, self.sealing_threads
                )),
                Cell::from(format!("{}/{}", item.pc1_running, self.pc1_concurrent)),
                Cell::from(format!("{}/{}", item.pc2_running, self.pc2_concurrent)),
                Cell::from(item.seed_waiting.to_string()),
                Cell::from(format!("{}/{}", item.c2_running, self.c2_concurrent)),
                Cell::from(item.finished_sectors.to_string()),
            ])
        });
        let t = Table::new(rows)
            .header(header)
            .block(
                Block::default()
                    .borders(Borders::ALL)
                    .title("sealing calculator"),
            )
            .highlight_style(selected_style)
            .widths(&[Constraint::Ratio(1, 7); 7]);
        f.render_stateful_widget(t, rects[0], &mut self.state);
    }
}

struct Terminal(TuiTerminal<CrosstermBackend<Stdout>>);

impl Terminal {
    fn inner_mut(&mut self) -> &mut TuiTerminal<CrosstermBackend<Stdout>> {
        &mut self.0
    }
}

impl Drop for Terminal {
    fn drop(&mut self) {
        // cleanup terminal
        disable_raw_mode().expect("Unable to disable raw mode.");
        execute!(
            self.0.backend_mut(),
            LeaveAlternateScreen,
            DisableMouseCapture
        )
        .expect("Unable to leave alternate screen.");
        self.0.show_cursor().expect("Unable to show cursor");
    }
}

fn setup_terminal() -> Terminal {
    enable_raw_mode().expect("Unable to enter raw mode.");
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture)
        .expect("Unable to enter alternate screen");
    let backend = CrosstermBackend::new(stdout);
    let tui_terminal =
        TuiTerminal::new(backend).expect("Couldn't create new terminal with backend");

    Terminal(tui_terminal)
}
